package changeset

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func gitOrSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	gitOrSkip(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestChanged_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if got != nil {
		t.Errorf("non-git dir = %v, want nil", got)
	}
}

func TestChanged_FreshRepoNoHEAD(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "tracked.txt", "a")
	writeFile(t, dir, "nested/also.txt", "b")
	runGit(t, dir, "add", "tracked.txt", "nested/also.txt")
	// No commit yet — HEAD does not exist.
	writeFile(t, dir, "untracked.txt", "c")

	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	want := []string{"nested/also.txt", "tracked.txt", "untracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestChanged_CleanRepoIsEmpty(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.txt", "a")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-q", "-m", "init")

	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("clean repo = %v, want empty", got)
	}
}

func TestChanged_ModifiedAndUntracked(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "tracked.txt", "v1")
	writeFile(t, dir, "keep.txt", "keep")
	runGit(t, dir, "add", "tracked.txt", "keep.txt")
	runGit(t, dir, "commit", "-q", "-m", "init")

	writeFile(t, dir, "tracked.txt", "v2") // modified working tree
	writeFile(t, dir, "new.txt", "n")      // untracked

	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	want := []string{"new.txt", "tracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestChanged_StagedNewFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "seed.txt", "seed")
	runGit(t, dir, "add", "seed.txt")
	runGit(t, dir, "commit", "-q", "-m", "init")

	writeFile(t, dir, "staged.txt", "s")
	runGit(t, dir, "add", "staged.txt")

	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	want := []string{"staged.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestChanged_ExcludedByGitignore(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, ".gitignore", "ignored.txt\n")
	runGit(t, dir, "add", ".gitignore")
	runGit(t, dir, "commit", "-q", "-m", "init")

	writeFile(t, dir, "ignored.txt", "x") // covered by .gitignore
	writeFile(t, dir, "visible.txt", "y") // genuinely untracked

	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	want := []string{"visible.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (gitignored files must not appear)", got, want)
	}
}

func TestChanged_ResultIsSortedAndDeduplicated(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "b.txt", "b")
	writeFile(t, dir, "a.txt", "a")
	runGit(t, dir, "add", "b.txt", "a.txt")
	runGit(t, dir, "commit", "-q", "-m", "init")

	// Modify both in the working tree.
	writeFile(t, dir, "a.txt", "a2")
	writeFile(t, dir, "b.txt", "b2")
	// And add an untracked file whose name sorts between them.
	writeFile(t, dir, "aa.txt", "n")

	got, err := Changed(dir)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	want := []string{"a.txt", "aa.txt", "b.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
