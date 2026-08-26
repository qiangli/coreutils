// Package patchcmd implements patch(1) per POSIX.1-2016
// (https://pubs.opengroup.org/onlinepubs/9699919799/utilities/patch.html):
// apply a unified, context, or normal (ed-listing) diff to one or more
// files. The parsing and hunk-placement engine lives in pkg/patch, which is
// a clean-room implementation — no code here or in pkg/patch is derived
// from GNU patch (GPLv3). See docs/patch-continuation-ledger.md for exactly
// which flags and behaviors this covers and which remain unimplemented;
// docs/reference-policy.md is the authority for how POSIX vs. GNU
// extensions are decided.
//
// coreutils continues to also ship the pinned GNU patch POSIX external
// provider (pkg/posixprovider, cmds/posixproviders) rather than retiring
// it: this applet's coverage is a documented subset (no -e/ed-script
// input, no -D/ifdef merges, no RCS/SCCS retrieval, no binary or
// rename-only git patches, a simplified fuzz/whitespace model), and the
// provider remains available wherever exact GNU behavior or a feature
// outside that subset is required.
package patchcmd

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/pkg/patch"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "patch",
	Synopsis: "Apply a diff file to one or more original files.",
	Usage: "patch [OPTION]... [ORIGFILE [PATCHFILE]]\n" +
		"Apply unified, context, or normal-format hunks from PATCHFILE\n" +
		"(default: standard input) to ORIGFILE (default: the name(s)\n" +
		"recorded in the patch headers).",
}

func init() { cmd.Run = run; tool.Register(cmd) }

const autoStripSentinel = -1

func run(rc *tool.RunContext, args []string) int {
	flags := tool.NewFlags(cmd.Name)
	strip := flags.IntP("strip", "p", autoStripSentinel, "strip NUM leading path components from file names (default: use the basename)")
	reverse := flags.BoolP("reverse", "R", false, "assume this patch was created with the old and new files swapped")
	force := flags.BoolP("force", "f", false, "assume the patch is not reversed or already applied, and apply it regardless")
	batch := flags.BoolP("batch", "t", false, "like --force: never treat a hunk as already applied")
	forward := flags.BoolP("forward", "N", false, "silently skip patches that appear reversed or already applied")
	fuzz := flags.IntP("fuzz", "F", 2, "set the fuzz factor (max context lines that may be dropped from a hunk's edges) to NUM")
	ignoreWS := flags.BoolP("ignore-whitespace", "l", false, "compare context/old lines ignoring whitespace differences")
	removeEmpty := flags.BoolP("remove-empty-files", "E", false, "remove output files that become empty after patching")
	quiet := flags.BoolP("quiet", "s", false, "work silently unless an error occurs")
	silent := flags.Bool("silent", false, "same as --quiet")
	backup := flags.BoolP("backup", "b", false, "back up the original contents of each file with a .orig suffix before patching")
	output := flags.StringP("output", "o", "", "output the patched file to FILE instead of patching in place")
	input := flags.StringP("input", "i", "", "read the patch from FILE instead of the PATCHFILE operand or standard input")
	rejectFile := flags.StringP("reject-file", "r", "", "name the reject file NAME instead of <file>.rej (only with a single-file patch)")
	directory := flags.StringP("directory", "d", "", "resolve file names relative to DIR instead of the current directory")
	unified := flags.BoolP("unified", "u", false, "interpret the patch as unified format (auto-detected by default)")
	context := flags.BoolP("context", "c", false, "interpret the patch as context format (auto-detected by default)")
	normal := flags.BoolP("normal", "n", false, "interpret the patch as normal format (auto-detected by default)")
	dryRun := flags.Bool("dry-run", false, "report what would happen without changing any file")
	edFlag := flags.BoolP("ed", "e", false, "interpret the patch as an ed script")
	ifdef := flags.StringP("ifdef", "D", "", "merge using #ifdef NAME instead of patching in place")
	get := flags.IntP("get", "g", autoStripSentinel, "get missing files from RCS/SCCS/... as needed")

	operands, code := tool.Parse(rc, cmd, flags, args)
	if code >= 0 {
		return code
	}

	if *edFlag {
		return tool.NotSupported(rc, cmd, "-e/--ed (ed-script diffs)")
	}
	if *ifdef != "" {
		return tool.NotSupported(rc, cmd, "-D/--ifdef")
	}
	if *get != autoStripSentinel {
		return tool.NotSupported(rc, cmd, "-g/--get (RCS/SCCS retrieval)")
	}

	formatHints := 0
	for _, b := range []bool{*unified, *context, *normal} {
		if b {
			formatHints++
		}
	}
	if formatHints > 1 {
		return tool.UsageError(rc, cmd, "only one of -u, -c, -n may be given")
	}

	if len(operands) > 2 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
	}
	var origFile, patchFileOperand string
	if len(operands) >= 1 {
		origFile = operands[0]
	}
	if len(operands) >= 2 {
		patchFileOperand = operands[1]
	}

	patchPath := *input
	if patchPath == "" {
		patchPath = patchFileOperand
	}
	var data []byte
	var err error
	if patchPath != "" {
		data, err = os.ReadFile(resolveOperandPath(rc, *directory, patchPath))
	} else {
		data, err = io.ReadAll(rc.In)
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
		return 2
	}

	parsed, err := patch.Parse(data)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 2
	}
	wantFormat := patch.Format(-1)
	switch {
	case *unified:
		wantFormat = patch.FormatUnified
	case *context:
		wantFormat = patch.FormatContext
	case *normal:
		wantFormat = patch.FormatNormal
	}
	if wantFormat >= 0 {
		for _, fp := range parsed.Files {
			if fp.Format != wantFormat {
				fmt.Fprintf(rc.Err, "%s: input is %s format, not requested %s format\n", cmd.Name, fp.Format, wantFormat)
				return 2
			}
		}
	}

	if *rejectFile != "" && len(parsed.Files) > 1 {
		return tool.UsageError(rc, cmd, "-r/--reject-file requires a patch that touches exactly one file")
	}
	if origFile != "" && len(parsed.Files) > 1 {
		return tool.UsageError(rc, cmd, "a FILE operand cannot be combined with a multi-file patch")
	}

	ro := runOptions{
		strip:       *strip,
		reverse:     *reverse,
		directory:   *directory,
		origFile:    origFile,
		output:      *output,
		rejectFile:  *rejectFile,
		quiet:       *quiet || *silent,
		backup:      *backup,
		removeEmpty: *removeEmpty,
		dryRun:      *dryRun,
		applyOpts: patch.ApplyOptions{
			Reverse:          *reverse,
			Fuzz:             *fuzz,
			IgnoreWhitespace: *ignoreWS,
			// POSIX's default interactive reversal question cannot be asked in
			// this non-interactive applet. Fail closed unless -N explicitly
			// requests forward/already-applied patches to be ignored.
			Force: *force || *batch || !*forward,
		},
	}

	trouble := false
	for _, fp := range parsed.Files {
		if fp.Unsupported != "" {
			fmt.Fprintf(rc.Err, "%s: %s: %s\n", cmd.Name, filePatchLabel(fp), fp.Unsupported)
			trouble = true
			continue
		}
		if fp.Format == patch.FormatNormal && origFile == "" {
			fmt.Fprintf(rc.Err, "%s: a normal-format diff needs an explicit ORIGFILE operand\n", cmd.Name)
			trouble = true
			continue
		}
		if !applyOneFile(rc, ro, fp) {
			trouble = true
		}
	}
	if trouble {
		return 1
	}
	return 0
}

type runOptions struct {
	strip       int
	reverse     bool
	directory   string
	origFile    string
	output      string
	rejectFile  string
	quiet       bool
	backup      bool
	removeEmpty bool
	dryRun      bool
	applyOpts   patch.ApplyOptions
}

func filePatchLabel(fp patch.FilePatch) string {
	switch {
	case fp.NewName != "" && fp.NewName != patch.DevNull:
		return fp.NewName
	case fp.OldName != "" && fp.OldName != patch.DevNull:
		return fp.OldName
	case fp.RenameTo != "":
		return fp.RenameTo
	case fp.RenameFrom != "":
		return fp.RenameFrom
	default:
		return "<unknown>"
	}
}

func describeErr(err error) string {
	if pe, ok := err.(*fs.PathError); ok {
		return fmt.Sprintf("%s: %v", pe.Path, pe.Err)
	}
	return err.Error()
}

func applyOneFile(rc *tool.RunContext, ro runOptions, fp patch.FilePatch) bool {
	oldName, newName := fp.OldName, fp.NewName
	if ro.reverse {
		oldName, newName = newName, oldName
	}
	createMode := oldName == patch.DevNull && newName != patch.DevNull
	deleteMode := newName == patch.DevNull && oldName != patch.DevNull

	displayName := oldName
	if createMode {
		displayName = newName
	}
	if displayName == "" || displayName == patch.DevNull {
		if oldName != "" && oldName != patch.DevNull {
			displayName = oldName
		}
	}

	var targetPath string
	if ro.origFile != "" {
		targetPath = resolveOperandPath(rc, ro.directory, ro.origFile)
	} else {
		targetPath, displayName = resolveHeaderTarget(rc, ro.directory, oldName, newName, ro.strip, createMode)
	}

	var oldLines []string
	var oldNoEOL bool
	existed := false
	var origMode fs.FileMode = 0o644
	if !createMode {
		fi, statErr := os.Stat(targetPath)
		if statErr != nil {
			fmt.Fprintf(rc.Err, "%s: can't find file to patch: %s\n", cmd.Name, describeErr(statErr))
			return false
		}
		origMode = fi.Mode().Perm()
		content, err := os.ReadFile(targetPath)
		if err != nil {
			fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
			return false
		}
		existed = true
		oldLines, oldNoEOL = splitFileLines(content)
	}

	res := patch.Apply(oldLines, oldNoEOL, fp.Hunks, ro.applyOpts)

	if !ro.quiet {
		verb := "patching file"
		if createMode {
			verb = "creating file"
		} else if deleteMode {
			verb = "removing file"
		}
		fmt.Fprintf(rc.Err, "%s %s %s\n", verb, displayName, dryRunSuffix(ro.dryRun))
	}
	failed := 0
	for _, r := range res.Reports {
		switch r.Outcome {
		case patch.HunkAppliedFuzzy:
			if !ro.quiet {
				fmt.Fprintf(rc.Err, "Hunk #%d succeeded at %d with fuzz %d.\n", r.Index, r.At+1, r.FuzzUsed)
			}
		case patch.HunkAlreadyApplied:
			if !ro.quiet {
				fmt.Fprintf(rc.Err, "Hunk #%d ignored -- already applied.\n", r.Index)
			}
		case patch.HunkFailed:
			failed++
			fmt.Fprintf(rc.Err, "Hunk #%d FAILED.\n", r.Index)
		}
	}
	allApplied := res.AllApplied()

	if ro.dryRun {
		return allApplied
	}

	finalEmpty := len(res.Lines) == 0
	removeFile := allApplied && ro.output == "" && (deleteMode || (ro.removeEmpty && existed && finalEmpty && !deleteMode))

	outPath := targetPath
	if ro.output != "" {
		outPath = resolveOperandPath(rc, ro.directory, ro.output)
	}
	outInfo, outStatErr := os.Stat(outPath)
	outExisted := outStatErr == nil
	if outStatErr != nil && !os.IsNotExist(outStatErr) {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(outStatErr))
		return false
	}

	switch {
	case removeFile:
		if ro.backup && existed {
			if err := backupFile(targetPath); err != nil {
				fmt.Fprintf(rc.Err, "%s: backup failed: %s\n", cmd.Name, describeErr(err))
				return false
			}
		}
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
			return false
		}
	default:
		if ro.backup && outExisted {
			if err := backupFile(outPath); err != nil {
				fmt.Fprintf(rc.Err, "%s: backup failed: %s\n", cmd.Name, describeErr(err))
				return false
			}
		}
		content := joinFileLines(res.Lines, res.NoFinalNewline)
		outMode := origMode
		if outExisted {
			outMode = outInfo.Mode().Perm()
		}
		if err := writeFileContent(rc, outPath, content, outMode, outExisted); err != nil {
			fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
			return false
		}
	}

	if !allApplied {
		rejPath := outPath + ".rej"
		if ro.rejectFile != "" {
			rejPath = resolveOperandPath(rc, ro.directory, ro.rejectFile)
		}
		rejData := patch.WriteReject(fp.OldName, fp.NewName, res.Rejects)
		if err := os.WriteFile(rejPath, rejData, 0o644); err != nil {
			fmt.Fprintf(rc.Err, "%s: could not write reject file: %s\n", cmd.Name, describeErr(err))
			return false
		}
		fmt.Fprintf(rc.Err, "%d out of %d hunks failed -- saving rejects to file %s\n", failed, len(fp.Hunks), rejPath)
	}
	return allApplied
}

func dryRunSuffix(dryRun bool) string {
	if dryRun {
		return "(dry run)"
	}
	return ""
}

func splitFileLines(content []byte) (lines []string, noFinalNewline bool) {
	s := string(content)
	if s == "" {
		return nil, false
	}
	trailingNL := strings.HasSuffix(s, "\n")
	parts := strings.Split(s, "\n")
	if trailingNL {
		parts = parts[:len(parts)-1]
	}
	return parts, !trailingNL
}

func joinFileLines(lines []string, noFinalNewline bool) []byte {
	if len(lines) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for i, l := range lines {
		buf.WriteString(l)
		if i < len(lines)-1 || !noFinalNewline {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

// resolveTargetName applies POSIX's default basename selection when -p is
// omitted. An explicit -p count preserves the requested leading components.
func resolveTargetName(rc *tool.RunContext, dir, name string, strip int, wantExisting bool) string {
	s := strip
	if s < 0 {
		s = strings.Count(filepath.ToSlash(name), "/")
	}
	return resolveOperandPath(rc, dir, stripComponents(name, s))
}

func resolveHeaderTarget(rc *tool.RunContext, dir, oldName, newName string, strip int, create bool) (string, string) {
	if create {
		return resolveTargetName(rc, dir, newName, strip, false), newName
	}
	for _, name := range []string{oldName, newName} {
		if name == "" || name == patch.DevNull {
			continue
		}
		path := resolveTargetName(rc, dir, name, strip, true)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, name
		}
	}
	name := oldName
	if name == "" || name == patch.DevNull {
		name = newName
	}
	return resolveTargetName(rc, dir, name, strip, true), name
}

func resolveOperandPath(rc *tool.RunContext, dir, name string) string {
	if filepath.IsAbs(name) {
		return rc.Path(name)
	}
	return rc.Path(filepath.Join(dir, name))
}

func stripComponents(name string, n int) string {
	comps := strings.Split(name, "/")
	if n <= 0 {
		return name
	}
	if n >= len(comps) {
		return comps[len(comps)-1]
	}
	return strings.Join(comps[n:], "/")
}

func backupFile(path string) error {
	backup := path + ".orig"
	if _, err := os.Stat(backup); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(backup, content, fi.Mode().Perm())
}

// writeFileContent replaces path's content. A pre-existing file is
// replaced atomically (temp file + rename in the same directory, GNU
// patch's own approach — see cmds/sed's editInPlace for the identical
// pattern) so a crash mid-write cannot leave a half-patched file; a new
// file is created directly, honoring the embedding shell's umask through
// rc.OpenFile.
func writeFileContent(rc *tool.RunContext, path string, content []byte, perm fs.FileMode, existed bool) error {
	if !existed {
		if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
			return err
		}
		f, err := rc.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return err
		}
		if _, err := f.Write(content); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".patch-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		tmp.Close()
		if !keep {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceExisting(tmpName, path); err != nil {
		return err
	}
	keep = true
	return nil
}

// replaceExisting renames src onto dst, working around Windows' refusal to
// rename over an existing file the same way cmds/sed's editInPlace does.
func replaceExisting(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	placeholder, err := os.CreateTemp(filepath.Dir(dst), ".patch-old-*")
	if err != nil {
		return err
	}
	old := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		return err
	}
	if err := os.Remove(old); err != nil {
		return err
	}
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	return os.Remove(old)
}
