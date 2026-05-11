package changeset

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Changed returns the set of files different from HEAD (staged or unstaged)
// plus untracked files. This is what the agent has touched since the last commit.
// Returns (nil, nil) if dir is not a git repository.
func Changed(dir string) ([]string, error) {
	if !inGitRepo(dir) {
		return nil, nil
	}
	seen := map[string]struct{}{}

	if hasHEAD(dir) {
		out, err := gitOut(dir, "diff", "--name-only", "HEAD")
		if err != nil {
			return nil, err
		}
		addLines(seen, out)
	} else {
		// No commits yet: tracked files are part of the changeset.
		out, err := gitOut(dir, "ls-files")
		if err != nil {
			return nil, err
		}
		addLines(seen, out)
	}

	out, err := gitOut(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	addLines(seen, out)

	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}

func inGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func hasHEAD(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, errOut.String())
	}
	return out.String(), nil
}

func addLines(set map[string]struct{}, s string) {
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		set[line] = struct{}{}
	}
}
