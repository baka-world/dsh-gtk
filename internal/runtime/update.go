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
func (s *Store) CheckForUpdate(ctx context.Context, channel string) (Update, error) {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "latest"
	}
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
