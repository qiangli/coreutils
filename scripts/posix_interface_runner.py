#!/usr/bin/env python3
import argparse
import csv
import hashlib
import json
import os
import re
import subprocess
import sys
import time
import fcntl
from collections import defaultdict
from pathlib import Path

def eprint(*args, **kwargs):
    print(*args, file=sys.stderr, **kwargs)

def get_git_revision(repo_path):
    cmd = ['git', 'rev-parse', 'HEAD']
    result = subprocess.run(cmd, cwd=repo_path, capture_output=True, text=True, check=True)
    return result.stdout.strip()

def get_go_version():
    cmd = ['go', 'version']
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    return result.stdout.strip()

def get_go_env():
    cmd = ['go', 'env', 'GOOS', 'GOARCH']
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    lines = result.stdout.strip().split('\n')
    return {"GOOS": lines[0], "GOARCH": lines[1]}

def get_file_sha256(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(65536), b''):
            h.update(chunk)
    return h.hexdigest()

class Runner:
    def __init__(self):
        self.roots = {
            'coreutils': os.path.abspath('.')
        }
        if 'POSIX_SH_EVIDENCE_ROOT' in os.environ:
            self.roots['sh'] = os.path.abspath(os.environ['POSIX_SH_EVIDENCE_ROOT'])
        if 'POSIX_BASHY_EVIDENCE_ROOT' in os.environ:
            self.roots['bashy'] = os.path.abspath(os.environ['POSIX_BASHY_EVIDENCE_ROOT'])

    def parse_references(self, evidence_str, expected_repos, require_prefix=False):
        if evidence_str == '-':
            return []
        refs = []
        for ref in evidence_str.split(';'):
            ref = ref.strip()
            if not ref:
                continue
            
            if require_prefix:
                m = re.match(r'^([a-z0-9_-]+):([^#]+)#(Test[A-Za-z0-9_]+)$', ref)
            else:
                m = re.match(r'^(?:([a-z0-9_-]+):)?([^#]+)#(Test[A-Za-z0-9_]+)$', ref)
                
            if not m:
                raise ValueError(f"Malformed reference: {ref}")
            
            if require_prefix:
                repo, path, test_id = m.groups()
            else:
                repo, path, test_id = m.groups()
                if not repo:
                    repo = 'coreutils'
                    
            if repo not in expected_repos:
                raise ValueError(f"Wrong-owner reference: {ref} (expected {expected_repos})")
            
            if repo not in self.roots:
                raise ValueError(f"Unavailable root for repo: {repo} in {ref}")
            
            refs.append({'repo': repo, 'path': path, 'test_id': test_id, 'raw': ref})
        return refs

def write_ledger(path, data):
    tmp = path.with_suffix('.tmp')
    with open(tmp, 'w') as f:
        json.dump(data, f, indent=2)
    os.rename(tmp, path)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('commands', nargs='*')
    parser.add_argument('--state-dir', required=True)
    parser.add_argument('--owner', choices=['go', 'shell'])
    parser.add_argument('--all', action='store_true')
    parser.add_argument('--dry-run', action='store_true')
    parser.add_argument('--json', action='store_true')
    args = parser.parse_args()

    tsv_path = 'docs/posix-required-command-interfaces.tsv'
    manifest_sha256 = get_file_sha256(tsv_path)
    
    commands_data = {}
    with open(tsv_path, newline='') as f:
        reader = csv.DictReader(f, delimiter='\t')
        for row in reader:
            owner = row.get('effective_owner')
            if owner not in ('go', 'shell'):
                continue
            commands_data[row['command']] = row
            
    selected_commands = []
    if args.commands:
        for c in args.commands:
            if c not in commands_data:
                eprint(f"Error: command {c} not found or not owned by go/shell")
                sys.exit(1)
            selected_commands.append(c)
    elif args.owner:
        selected_commands = [c for c, d in commands_data.items() if d['effective_owner'] == args.owner]
    elif args.all:
        selected_commands = list(commands_data.keys())
    else:
        eprint("Error: must provide commands, --owner, or --all")
        sys.exit(1)
        
    if not selected_commands:
        eprint("Error: empty selection")
        sys.exit(1)
        
    selected_commands.sort()
    
    selection_dict = {}
    used_roots = set()
    runner = Runner()
    
    for c in selected_commands:
        row = commands_data[c]
        owner = row['effective_owner']
        refs = []
        try:
            if owner == 'go':
                go_refs = runner.parse_references(row['go_evidence'], ['coreutils'])
                refs.extend(go_refs)
            elif owner == 'shell':
                sh_refs = runner.parse_references(row['shell_evidence'], ['sh', 'bashy'], require_prefix=True)
                bashy_refs = runner.parse_references(row['shell_routing_evidence'], ['sh', 'bashy'], require_prefix=True)
                refs.extend(sh_refs)
                refs.extend(bashy_refs)
        except ValueError as e:
            eprint(f"Error parsing evidence for {c}: {e}")
            sys.exit(1)
            
        if not refs:
            eprint(f"Error: missing evidence for {c}")
            sys.exit(1)
            
        raw_refs = [r['raw'] for r in refs]
        if len(raw_refs) != len(set(raw_refs)):
            eprint(f"Error: duplicate references for {c}")
            sys.exit(1)
            
        selection_dict[c] = refs
        for r in refs:
            used_roots.add(r['repo'])
            
    git_revisions = {}
    for r in used_roots:
        git_revisions[r] = get_git_revision(runner.roots[r])
        
    go_env = get_go_env()
    selection_hash_data = {c: sorted([r['raw'] for r in refs]) for c, refs in selection_dict.items()}
    
    contract = {
        "schema_version": "1.0",
        "manifest_sha256": manifest_sha256,
        "selection": selection_hash_data,
        "git_revisions": git_revisions,
        "go_version": get_go_version(),
        "go_env": go_env,
        "posixly_correct": "1"
    }
    
    contract_json = json.dumps(contract, sort_keys=True)
    contract_hash = hashlib.sha256(contract_json.encode('utf-8')).hexdigest()
    
    state_dir = Path(args.state_dir)
    state_dir.mkdir(parents=True, exist_ok=True)
    lock_file = state_dir / 'ledger.lock'
    ledger_file = state_dir / 'ledger.json'
    
    with open(lock_file, 'w') as lf:
        fcntl.flock(lf, fcntl.LOCK_EX)
        
        ledger = {}
        if ledger_file.exists():
            with open(ledger_file, 'r') as f:
                try:
                    ledger = json.load(f)
                except json.JSONDecodeError:
                    pass
                    
        if ledger.get('contract_hash') != contract_hash:
            ledger = {
                'contract_hash': contract_hash,
                'contract': contract,
                'results': {}
            }
            
        any_failure = False
        
        for c in selected_commands:
            res = ledger['results'].get(c, {})
            if res.get('pass') is True and not res.get('incomplete'):
                if args.json:
                    print(json.dumps({'command': c, 'status': 'skipped'}))
                else:
                    print(f"Skipping {c} (prior success)")
                continue
                
            refs = selection_dict[c]
            groups = defaultdict(list)
            for r in refs:
                pkg = os.path.dirname(r['path'])
                if not pkg:
                    pkg = "."
                elif not pkg.startswith('.'):
                    pkg = "./" + pkg
                groups[(r['repo'], pkg)].append(r['test_id'])
                
            argvs = []
            for (repo, pkg), test_ids in sorted(groups.items()):
                test_regex = "^(" + "|".join(test_ids) + ")$"
                argv = ["go", "test", "-v", "-run", test_regex, pkg]
                argvs.append({
                    "repo": repo,
                    "cwd": runner.roots[repo],
                    "cmd": argv
                })
                
            attempt = res.get('attempt', 0) + 1
            start_ts = time.time()
            
            ledger['results'][c] = {
                'attempt': attempt,
                'start_timestamp': start_ts,
                'incomplete': True
            }
            write_ledger(ledger_file, ledger)
            
            c_pass = True
            c_argvs = []
            stdout_hasher = hashlib.sha256()
            stderr_hasher = hashlib.sha256()
            final_exit = 0
            
            if args.json:
                print(json.dumps({'command': c, 'status': 'running', 'attempt': attempt}))
            else:
                print(f"Running {c} (attempt {attempt})...")
                
            for group in argvs:
                c_argvs.append(group['cmd'])
                if args.dry_run:
                    continue
                    
                env = os.environ.copy()
                env['POSIXLY_CORRECT'] = '1'
                p = subprocess.Popen(
                    group['cmd'],
                    cwd=group['cwd'],
                    env=env,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE
                )
                out, err = p.communicate()
                stdout_hasher.update(out)
                stderr_hasher.update(err)
                
                if p.returncode != 0:
                    c_pass = False
                    final_exit = p.returncode
                    break
                    
                if len(out) == 0:
                    c_pass = False
                    final_exit = 1
                    break
            
            if not args.dry_run:
                ledger['results'][c].update({
                    'end_timestamp': time.time(),
                    'argv': c_argvs,
                    'exit_status': final_exit,
                    'stdout_sha256': stdout_hasher.hexdigest(),
                    'stderr_sha256': stderr_hasher.hexdigest(),
                    'pass': c_pass
                })
                del ledger['results'][c]['incomplete']
                write_ledger(ledger_file, ledger)
                
            if not c_pass and not args.dry_run:
                any_failure = True
                if args.json:
                    print(json.dumps({'command': c, 'status': 'failed'}))
                else:
                    print(f"Failed {c}")
            else:
                if args.json:
                    print(json.dumps({'command': c, 'status': 'passed'}))
                else:
                    print(f"Passed {c}")
                    
        if any_failure:
            sys.exit(1)

if __name__ == '__main__':
    main()
