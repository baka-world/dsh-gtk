package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Update is what a check found on the npm registry.
type Update struct {
	// Channel is the dist-tag checked, such as "latest" or "alpha".
	Channel string `json:"channel"`
	// Version is the version that channel names.
	Version string `json:"version"`
	// CheckedAt is an RFC 3339 time.
	CheckedAt string `json:"checkedAt"`
}

// CachedUpdate returns the last check, or nil when none ran.
func (s *Store) CachedUpdate() (*Update, error) {
	var u Update
	err := readJSON(s.updatePath(), &u)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if u.Version == "" {
		return nil, nil
	}
	return &u, nil
}

// CheckForUpdate asks npm, through the registry npm is configured with, for
// the version of the dist-tag channel, and caches the answer.
//
// An empty channel follows the tree the app runs: the tag that names current,
// else the tag with the newest version. dsh publishes prereleases under tags
// of their own ("alpha", "next") while "latest" trails them, so a fixed
// default would either hide the prerelease someone installed or answer "up to
// date" for a tree that is newer than "latest".
func (s *Store) CheckForUpdate(ctx context.Context, channel, current string) (Update, error) {
	channel = strings.TrimSpace(channel)
	auto := channel == ""
	if s.Node == "" || s.NPM == "" {
		return Update{}, errors.New("checking for updates needs node and npm; run scripts/prepare-runtime.sh")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Node, s.NPM, "view", "@deepseek-ai/dsh", "dist-tags", "--json", "--loglevel=error")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return Update{}, fmt.Errorf("npm view: %w\n%s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return Update{}, fmt.Errorf("npm view: %w", err)
	}
	var tags map[string]string
	if err := json.Unmarshal(out, &tags); err != nil {
		return Update{}, fmt.Errorf("npm view returned %q: %w", strings.TrimSpace(string(out)), err)
	}
	if auto {
		var err error
		if channel, err = PickChannel(tags, current); err != nil {
			return Update{}, err
		}
	}
	version := tags[channel]
	if version == "" {
		names := make([]string, 0, len(tags))
		for name := range tags {
			names = append(names, name)
		}
		sort.Strings(names)
		return Update{}, fmt.Errorf("no %q dist-tag; the registry has: %s", channel, strings.Join(names, ", "))
	}
	u := Update{Channel: channel, Version: version, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeJSON(s.updatePath(), u); err != nil {
		return u, fmt.Errorf("cache the check: %w", err)
	}
	return u, nil
}

// updatePath is the cached check, next to the selection.
func (s *Store) updatePath() string {
	return filepath.Join(filepath.Dir(s.State), "runtime-update.json")
}

// PickChannel returns the dist-tag to follow for a tree at version current:
// the tag that names that version, else the tag with the newest version.
// Ties between tags that name the same version go to the more stable channel,
// so a release that also happens to be "latest" follows "latest" and not a
// prerelease tag that still points at it.
func PickChannel(tags map[string]string, current string) (string, error) {
	if len(tags) == 0 {
		return "", errors.New("the registry named no dist-tags")
	}
	var exact []string
	for name, version := range tags {
		if current == "" {
			continue
		}
		if n, ok := CompareVersions(version, current); ok && n == 0 {
			exact = append(exact, name)
		}
	}
	if len(exact) > 0 {
		sortTags(exact)
		return exact[0], nil
	}
	best, bestVersion := "", ""
	for name, version := range tags {
		if best == "" {
			best, bestVersion = name, version
			continue
		}
		if n, ok := CompareVersions(version, bestVersion); ok && n > 0 {
			best, bestVersion = name, version
		}
	}
	if best == "" {
		return "", fmt.Errorf("no version among the dist-tags: %v", tags)
	}
	return best, nil
}

// tagOrder ranks the channels dsh publishes, most stable first; unknown tags
// sort after them by name.
var tagOrder = []string{"latest", "next", "rc", "beta", "alpha"}

func sortTags(names []string) {
	sort.SliceStable(names, func(i, j int) bool {
		ri, rj := tagRank(names[i]), tagRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
}

func tagRank(name string) int {
	for i, known := range tagOrder {
		if name == known {
			return i
		}
	}
	return len(tagOrder)
}

// IsNewer reports whether candidate is a later version than current.
func IsNewer(candidate, current string) bool {
	n, ok := CompareVersions(candidate, current)
	return ok && n > 0
}

// CompareVersions compares semantic versions, prerelease identifiers and a
// leading v included, and returns -1, 0 or 1. ok is false when either is not
// a version.
func CompareVersions(a, b string) (int, bool) {
	va, ok := parseSemver(a)
	if !ok {
		return 0, false
	}
	vb, ok := parseSemver(b)
	if !ok {
		return 0, false
	}
	for i := range va.nums {
		if va.nums[i] != vb.nums[i] {
			if va.nums[i] < vb.nums[i] {
				return -1, true
			}
			return 1, true
		}
	}
	// A release is later than any prerelease of the same version.
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0, true
	case len(va.pre) == 0:
		return 1, true
	case len(vb.pre) == 0:
		return -1, true
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		x, y := va.pre[i], vb.pre[i]
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				if xn < yn {
					return -1, true
				}
				return 1, true
			}
		case xerr == nil:
			return -1, true // numeric identifiers sort before alphanumeric
		case yerr == nil:
			return 1, true
		default:
			if x != y {
				if x < y {
					return -1, true
				}
				return 1, true
			}
		}
	}
	switch {
	case len(va.pre) < len(vb.pre):
		return -1, true
	case len(va.pre) > len(vb.pre):
		return 1, true
	}
	return 0, true
}

type semver struct {
	nums [3]int
	pre  []string
}

func parseSemver(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var out semver
	if i := strings.IndexByte(v, '-'); i >= 0 {
		out.pre = strings.Split(v[i+1:], ".")
		v = v[:i]
		for _, p := range out.pre {
			if p == "" {
				return semver{}, false
			}
		}
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		out.nums[i] = n
	}
	return out, true
}
