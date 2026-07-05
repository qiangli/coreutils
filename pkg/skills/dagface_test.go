package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// tasksFixture is a real dag task file: targets are ### children of a
// `## Tasks` section (the dag parser's rule), bodies are fenced bash.
func tasksFixture(work string) string {
	flag := filepath.ToSlash(filepath.Join(work, "made.txt"))
	return "# demo pipeline\n\n## Tasks\n\n### hello\n\nWrites the marker.\n\n```bash\necho made > " + flag + "\n```\n\n### boom\n\nAlways fails.\n\n```bash\nexit 3\n```\n"
}

func writeTasksSkill(t *testing.T, root, name, frontmatter, canon, tasks string) string {
	t.Helper()
	dir := writeSkillDir(t, root, name, frontmatter, canon)
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte(tasks), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Uncontracted skill with a tasks face: the target executes through the
// dag engine, plainly (no attestation — rung "tasks").
func TestRunTargetUncontracted(t *testing.T) {
	store, work := t.TempDir(), t.TempDir()
	src := t.TempDir()
	dir := writeTasksSkill(t, src, "taskful",
		"---\nname: taskful\ndescription: has targets\n---", "", tasksFixture(work))
	f := &cobraRunner{t: t, opts: []Option{WithConfigDir(store)}}
	if _, _, err := f.run("add", dir); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := f.run("run", "taskful", "--target", "hello")
	if err != nil {
		t.Fatalf("target hello: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "ok: true") {
		t.Fatalf("stdout: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(work, "made.txt")); err != nil {
		t.Fatal("target did not run")
	}
	// No attestation for the uncontracted rung.
	if _, err := os.Stat(filepath.Join(store, "attest", "taskful.jsonl")); err == nil {
		t.Fatal("uncontracted target run attested")
	}
	// A failing target propagates.
	if _, _, err := f.run("run", "taskful", "--target", "boom"); err == nil {
		t.Fatal("boom target exited 0")
	}
	// Unknown target errors with the inventory.
	_, _, err = f.run("run", "taskful", "--target", "nope")
	if err == nil || !strings.Contains(err.Error(), "hello") {
		t.Fatalf("unknown target: %v", err)
	}
	// --target without a tasks face is a clear error.
	plain := writeSkillDir(t, src, "no-tasks", "---\nname: no-tasks\ndescription: x\n---", "")
	if _, _, err := f.run("add", plain); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.run("run", "no-tasks", "--target", "hello"); err == nil || !strings.Contains(err.Error(), "tasks.md") {
		t.Fatalf("no-tasks: %v", err)
	}
	// --target + --adapt refuse to combine.
	if _, _, err := f.run("run", "taskful", "--target", "hello", "--adapt"); err == nil {
		t.Fatal("--target --adapt combined")
	}
}

// Contracted skill: the dag target runs AS the steps phase — contract
// evaluated, effects observed, receipt attested.
func TestRunTargetContracted(t *testing.T) {
	store, work := t.TempDir(), t.TempDir()
	flag := filepath.ToSlash(filepath.Join(work, "made.txt"))
	src := t.TempDir()
	dir := writeTasksSkill(t, src, "taskful",
		"---\nname: taskful\ndescription: contracted targets\nmetadata:\n  check-tests: \"test -f "+flag+"\"\n---",
		"sokilili demo efefecato reada wurite fini enisure gereeni fini fini\n",
		tasksFixture(work))
	f := &cobraRunner{t: t, opts: []Option{WithConfigDir(store)}}
	if _, _, err := f.run("add", dir); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := f.run("run", "taskful", "--target", "hello")
	if err != nil {
		t.Fatalf("target: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "valid: true") || !strings.Contains(stdout, "outcome: target:hello") ||
		!strings.Contains(stdout, "effects: read write") {
		t.Fatalf("receipt: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(store, "attest", "taskful.jsonl")); err != nil {
		t.Fatal("contracted target run not attested")
	}
}

// The effect cap still governs: a read-only cap refuses a target run at
// pre-flight (targets are read+write by convention).
func TestRunTargetCapRefusal(t *testing.T) {
	store, work := t.TempDir(), t.TempDir()
	src := t.TempDir()
	dir := writeTasksSkill(t, src, "narrow",
		"---\nname: narrow\ndescription: read-only cap\nmetadata:\n  check-tests: \"true\"\n---",
		"sokilili demo efefecato reada fini enisure gereeni fini fini\n",
		tasksFixture(work))
	f := &cobraRunner{t: t, opts: []Option{WithConfigDir(store)}}
	if _, _, err := f.run("add", dir); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.run("run", "narrow", "--target", "hello")
	if err == nil || !strings.Contains(err.Error(), "pre-flight") {
		t.Fatalf("cap refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "made.txt")); err == nil {
		t.Fatal("refused target still ran")
	}
}

// Embedded skills (no on-disk dir) materialize tasks.md to a temp file.
func TestRunTargetEmbedded(t *testing.T) {
	work := t.TempDir()
	embedded := fstest.MapFS{
		"taskful/SKILL.md": {Data: []byte("---\nname: taskful\ndescription: embedded targets\n---\nbody\n")},
		"taskful/tasks.md": {Data: []byte(tasksFixture(work))},
	}
	f := &cobraRunner{t: t, opts: []Option{
		WithSource(EmbedSource(embedded, RingEmbedded)),
		WithConfigDir(t.TempDir()),
	}}
	stdout, _, err := f.run("run", "taskful", "--target", "hello")
	if err != nil {
		t.Fatalf("embedded target: %v\n%s", err, stdout)
	}
	if _, err := os.Stat(filepath.Join(work, "made.txt")); err != nil {
		t.Fatal("embedded target did not run")
	}
}
