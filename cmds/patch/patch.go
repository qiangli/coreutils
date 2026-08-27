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
// This pure-Go applet is the sole shipped patch owner. It implements all four
// POSIX input forms, including diff -e scripts and -D conditional merges.
// Non-POSIX RCS/SCCS retrieval, binary patches, and rename-only Git patches
// remain fail-closed. There is no external-provider fallback.
package patchcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	corelocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/patch"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "patch",
	Synopsis: "Apply a diff file to one or more original files.",
	Usage: "patch [OPTION]... [ORIGFILE [PATCHFILE]]\n" +
		"Apply unified, context, normal, or ed-format changes from PATCHFILE\n" +
		"(default: standard input) to ORIGFILE (default: the name(s)\n" +
		"recorded in the patch headers).",
}

func init() { cmd.Run = run; tool.Register(cmd) }

const autoStripSentinel = -1

var cIdentifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var readPromptLine = func() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer tty.Close()
	line, err := bufio.NewReader(tty).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

func askPrompt(rc *tool.RunContext, prompt string, stdout bool) (string, error) {
	w := rc.Err
	if stdout {
		w = rc.Out
	}
	if _, err := io.WriteString(w, prompt); err != nil {
		return "", err
	}
	return readPromptLine()
}

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

	if *get != autoStripSentinel {
		return tool.NotSupported(rc, cmd, "-g/--get (RCS/SCCS retrieval)")
	}

	formatHints := 0
	for _, b := range []bool{*unified, *context, *normal, *edFlag} {
		if b {
			formatHints++
		}
	}
	if formatHints > 1 {
		return tool.UsageError(rc, cmd, "only one of -u, -c, -n, -e may be given")
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
	if *ifdef != "" && !cIdentifierRE.MatchString(*ifdef) {
		return tool.UsageError(rc, cmd, "invalid -D define %q", *ifdef)
	}
	if *edFlag {
		if *reverse {
			return tool.UsageError(rc, cmd, "-R cannot be used with -e")
		}
		if *ifdef != "" {
			return tool.UsageError(rc, cmd, "-D cannot be used with -e")
		}
		return runEdScript(rc, data, origFile, *directory, *output, *backup, *dryRun)
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
		strip:         *strip,
		reverse:       *reverse,
		directory:     *directory,
		origFile:      origFile,
		output:        *output,
		rejectFile:    *rejectFile,
		quiet:         *quiet || *silent,
		backup:        *backup,
		removeEmpty:   *removeEmpty,
		dryRun:        *dryRun,
		force:         *force,
		batch:         *batch,
		forward:       *forward,
		define:        *ifdef,
		backedUp:      make(map[string]bool),
		intermediate:  make(map[string][]byte),
		rejectStarted: make(map[string]bool),
		reverseState:  new(bool),
		reverseProbe:  func() *bool { v := true; return &v }(),
		applyOpts: patch.ApplyOptions{
			Reverse:          *reverse,
			Fuzz:             *fuzz,
			IgnoreWhitespace: *ignoreWS,
			// The policy layer forces the initial normal attempt so a mismatch
			// can be distinguished from an already-applied/reversed portion.
			// -N instead permits the ordinary already-applied classification.
			Force: *force || *batch || !*forward,
		},
	}
	if *output != "" {
		ro.aggregate = &bytes.Buffer{}
	}

	trouble := false
	for _, fp := range parsed.Files {
		if fp.Unsupported != "" {
			fmt.Fprintf(rc.Err, "%s: %s: %s\n", cmd.Name, filePatchLabel(fp), fp.Unsupported)
			trouble = true
			continue
		}
		if !applyOneFile(rc, ro, fp) {
			trouble = true
		}
	}
	if ro.aggregate != nil && !ro.dryRun {
		if !writeAggregateOutput(rc, ro, ro.aggregate.Bytes()) {
			trouble = true
		}
	}
	if trouble {
		return 1
	}
	return 0
}

type runOptions struct {
	strip         int
	reverse       bool
	directory     string
	origFile      string
	output        string
	rejectFile    string
	quiet         bool
	backup        bool
	removeEmpty   bool
	dryRun        bool
	force         bool
	batch         bool
	forward       bool
	define        string
	backedUp      map[string]bool
	intermediate  map[string][]byte
	rejectStarted map[string]bool
	reverseState  *bool
	reverseProbe  *bool
	aggregate     *bytes.Buffer
	applyOpts     patch.ApplyOptions
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

func runEdScript(rc *tool.RunContext, script []byte, origFile, directory, output string, backup, dryRun bool) int {
	if origFile == "" {
		var err error
		origFile, err = askPrompt(rc, "File to patch: ", true)
		if err != nil || origFile == "" {
			fmt.Fprintf(rc.Err, "%s: cannot determine file to patch: %v\n", cmd.Name, err)
			return 1
		}
	}
	target := resolveOperandPath(rc, directory, origFile)
	content, err := os.ReadFile(target)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
		return 1
	}
	result, err := patch.ApplyEd(content, script)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}
	if dryRun {
		return 0
	}
	outPath := target
	if output != "" {
		outPath = resolveOperandPath(rc, directory, output)
	}
	fi, statErr := os.Stat(outPath)
	existed := statErr == nil
	mode := fs.FileMode(0o644)
	if existed {
		mode = fi.Mode().Perm()
	}
	if backup && existed {
		if err := backupFile(outPath); err != nil {
			fmt.Fprintf(rc.Err, "%s: backup failed: %s\n", cmd.Name, describeErr(err))
			return 1
		}
	}
	if err := writeFileContent(rc, outPath, result, mode, existed); err != nil {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
		return 1
	}
	return 0
}

func writeAggregateOutput(rc *tool.RunContext, ro runOptions, content []byte) bool {
	path := resolveOperandPath(rc, ro.directory, ro.output)
	fi, err := os.Stat(path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
		return false
	}
	mode := fs.FileMode(0o644)
	if existed {
		mode = fi.Mode().Perm()
	}
	if ro.backup && existed {
		if err := backupOnce(path, ro.backedUp); err != nil {
			fmt.Fprintf(rc.Err, "%s: backup failed: %s\n", cmd.Name, describeErr(err))
			return false
		}
	}
	if err := writeFileContent(rc, path, content, mode, existed); err != nil {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
		return false
	}
	return true
}

func applyOneFile(rc *tool.RunContext, ro runOptions, fp patch.FilePatch) bool {
	if ro.reverseState != nil && *ro.reverseState {
		ro.reverse = true
		ro.applyOpts.Reverse = true
	}
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
		var found bool
		targetPath, displayName, found = resolveHeaderTarget(rc, ro.directory, oldName, newName, fp.IndexName, ro.strip, createMode, ro.intermediate)
		if !found && !createMode {
			if ro.force || ro.batch {
				fmt.Fprintf(rc.Err, "%s: can't find file to patch\n", cmd.Name)
				return false
			}
			answer, err := askPrompt(rc, "File to patch: ", true)
			if err != nil || answer == "" {
				fmt.Fprintf(rc.Err, "%s: cannot determine file to patch: %v\n", cmd.Name, err)
				return false
			}
			targetPath = resolveOperandPath(rc, ro.directory, answer)
			displayName = answer
		}
	}

	var oldLines []string
	var oldNoEOL bool
	existed := false
	var origMode fs.FileMode = 0o644
	// A creation patch still has to inspect an existing destination.  Its
	// contents are what let the normal first-hunk policy recognize a patch
	// that was already applied and offer the POSIX reverse treatment.
	if !createMode {
		content, cached := ro.intermediate[targetPath]
		if !cached {
			fi, statErr := os.Stat(targetPath)
			if statErr != nil {
				fmt.Fprintf(rc.Err, "%s: can't find file to patch: %s\n", cmd.Name, describeErr(statErr))
				return false
			}
			origMode = fi.Mode().Perm()
			var err error
			content, err = os.ReadFile(targetPath)
			if err != nil {
				fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(err))
				return false
			}
		}
		existed = true
		oldLines, oldNoEOL = splitFileLines(content)
	} else if content, cached := ro.intermediate[targetPath]; cached {
		existed = true
		oldLines, oldNoEOL = splitFileLines(content)
	} else if fi, statErr := os.Stat(targetPath); statErr == nil {
		if fi.IsDir() {
			fmt.Fprintf(rc.Err, "%s: can't patch directory %s\n", cmd.Name, displayName)
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
	} else if !os.IsNotExist(statErr) {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(statErr))
		return false
	}

	creationConflict := createMode && (len(oldLines) != 0 || oldNoEOL)
	res, autoReversed := applyWithPolicy(rc, oldLines, oldNoEOL, fp, ro, creationConflict)
	if autoReversed {
		oldName, newName = newName, oldName
		createMode = oldName == patch.DevNull && newName != patch.DevNull
		deleteMode = newName == patch.DevNull && oldName != patch.DevNull
		if ro.reverseState != nil {
			*ro.reverseState = true
		}
	}

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
	if allApplied && ro.reverseProbe != nil {
		*ro.reverseProbe = false
	}

	if ro.dryRun {
		return allApplied
	}

	finalEmpty := len(res.Lines) == 0
	removeFile := allApplied && ro.output == "" && ro.define == "" && (deleteMode || (ro.removeEmpty && existed && finalEmpty && !deleteMode))

	outPath := targetPath
	if ro.aggregate != nil {
		content := joinFileLines(res.Lines, res.NoFinalNewline)
		ro.intermediate[targetPath] = append([]byte(nil), content...)
		_, _ = ro.aggregate.Write(content)
	}
	outInfo, outStatErr := os.Stat(outPath)
	outExisted := outStatErr == nil
	if outStatErr != nil && !os.IsNotExist(outStatErr) {
		fmt.Fprintf(rc.Err, "%s: %s\n", cmd.Name, describeErr(outStatErr))
		return false
	}

	switch {
	case ro.aggregate != nil:
		// The complete -o stream is committed once, after all file sections
		// have contributed in patch order. Originals are never modified.
	case removeFile:
		if ro.backup && existed {
			if err := backupOnce(targetPath, ro.backedUp); err != nil {
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
			if err := backupOnce(outPath, ro.backedUp); err != nil {
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
		if ro.output != "" {
			rejPath = resolveOperandPath(rc, ro.directory, ro.output) + ".rej"
		}
		if ro.rejectFile != "" {
			rejPath = resolveOperandPath(rc, ro.directory, ro.rejectFile)
		}
		rejData := patch.WriteRejectFormat(oldName, newName, fp.Format, res.Rejects)
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if ro.rejectStarted[rejPath] {
			flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		}
		rej, err := rc.OpenFile(rejPath, flags, 0o644)
		if err == nil {
			_, err = rej.Write(rejData)
			if closeErr := rej.Close(); err == nil {
				err = closeErr
			}
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "%s: could not write reject file: %s\n", cmd.Name, describeErr(err))
			return false
		}
		ro.rejectStarted[rejPath] = true
		fmt.Fprintf(rc.Err, "%d out of %d hunks failed -- saving rejects to file %s\n", failed, len(fp.Hunks), rejPath)
	}
	return allApplied
}

func applyWithPolicy(rc *tool.RunContext, oldLines []string, oldNoEOL bool, fp patch.FilePatch, ro runOptions, creationConflict bool) (patch.Result, bool) {
	apply := func(opts patch.ApplyOptions) patch.Result {
		if ro.define != "" {
			return patch.ApplyIfdef(oldLines, oldNoEOL, fp.Hunks, opts, ro.define)
		}
		return patch.Apply(oldLines, oldNoEOL, fp.Hunks, opts)
	}
	if ro.reverse || ro.force || ro.batch {
		return apply(ro.applyOpts), false
	}
	if ro.forward {
		if !creationConflict {
			return apply(ro.applyOpts), false
		}
		reverseOpts := ro.applyOpts
		reverseOpts.Reverse = true
		if reversed := apply(reverseOpts); reversed.AllApplied() {
			reports := make([]patch.HunkReport, len(fp.Hunks))
			for i := range reports {
				reports[i] = patch.HunkReport{Index: i + 1, Outcome: patch.HunkAlreadyApplied}
			}
			return patch.Result{Lines: append([]string(nil), oldLines...), NoFinalNewline: oldNoEOL, Reports: reports}, false
		}
		return failedResult(oldLines, oldNoEOL, fp.Hunks), false
	}
	forwardOpts := ro.applyOpts
	forwardOpts.Force = true
	forward := failedResult(oldLines, oldNoEOL, fp.Hunks)
	if !creationConflict {
		forward = apply(forwardOpts)
	}
	if len(forward.Reports) == 0 || forward.Reports[0].Outcome != patch.HunkFailed {
		return forward, false
	}
	if ro.reverseProbe != nil && !*ro.reverseProbe {
		return forward, false
	}
	reverseOpts := forwardOpts
	reverseOpts.Reverse = true
	reversed := apply(reverseOpts)
	if !reversed.AllApplied() {
		return forward, false
	}
	answer, err := askPrompt(rc, "Reversed (or previously applied) patch detected! Assume -R? [n] ", false)
	yes, matchErr := corelocale.MatchAffirmative(rc.Env, answer)
	if err == nil && matchErr == nil && yes {
		return reversed, true
	}
	if matchErr != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, matchErr)
	}
	return forward, false
}

func failedResult(oldLines []string, oldNoEOL bool, hunks []patch.Hunk) patch.Result {
	reports := make([]patch.HunkReport, len(hunks))
	for i := range reports {
		reports[i] = patch.HunkReport{Index: i + 1, Outcome: patch.HunkFailed}
	}
	return patch.Result{
		Lines:          append([]string(nil), oldLines...),
		NoFinalNewline: oldNoEOL,
		Reports:        reports,
		Rejects:        append([]patch.Hunk(nil), hunks...),
	}
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

func resolveHeaderTarget(rc *tool.RunContext, dir, oldName, newName, indexName string, strip int, create bool, intermediate map[string][]byte) (string, string, bool) {
	for _, name := range []string{oldName, newName, indexName} {
		if name == "" || name == patch.DevNull {
			continue
		}
		path := resolveTargetName(rc, dir, name, strip, true)
		if _, ok := intermediate[path]; ok {
			return path, name, true
		}
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, name, true
		}
	}
	if create {
		return resolveTargetName(rc, dir, newName, strip, false), newName, true
	}
	name := oldName
	if name == "" || name == patch.DevNull {
		name = newName
	}
	if (name == "" || name == patch.DevNull) && indexName != "" {
		name = indexName
	}
	if name == "" || name == patch.DevNull {
		return "", "", false
	}
	return resolveTargetName(rc, dir, name, strip, true), name, false
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

func backupOnce(path string, backedUp map[string]bool) error {
	if backedUp[path] {
		return nil
	}
	if err := backupFile(path); err != nil {
		return err
	}
	backedUp[path] = true
	return nil
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
