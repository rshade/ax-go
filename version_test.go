package ax

import (
	"runtime/debug"
	"strconv"
	"testing"
)

const (
	testInjectedVersion = "v1.2.3"
	testRevision        = "abcdef123456"
	testRevisionDirty   = testRevision + buildInfoVCSDirtySuffix
)

func testBuildInfo(mainVersion string, settings map[string]string) *debug.BuildInfo {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: mainVersion},
	}
	for key, value := range settings {
		info.Settings = append(info.Settings, debug.BuildSetting{
			Key:   key,
			Value: value,
		})
	}
	return info
}

func TestResolveVersionFrom(t *testing.T) {
	tests := []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		ok       bool
		want     string
	}{
		{
			name: "sentinel when build info unavailable",
			want: versionUnknown,
		},
		{
			name:     "injected version wins",
			injected: testInjectedVersion,
			info: testBuildInfo(buildInfoMainVersionDev, map[string]string{
				buildInfoVCSRevisionKey: "abc123",
				buildInfoVCSModifiedKey: strconv.FormatBool(true),
			}),
			ok:   true,
			want: testInjectedVersion,
		},
		{
			name:     "injected dirty describe wins",
			injected: "5bf9b77-dirty",
			ok:       false,
			want:     "5bf9b77-dirty",
		},
		{
			name:     "bare dev injection falls back to build info",
			injected: versionDev,
			info:     testBuildInfo(testInjectedVersion, nil),
			ok:       true,
			want:     testInjectedVersion,
		},
		{
			name:     "bare unknown injection falls back to sentinel",
			injected: versionBareUnknown,
			ok:       false,
			want:     versionUnknown,
		},
		{
			name:     "injected sentinel falls back to build info",
			injected: versionUnknown,
			info:     testBuildInfo(testInjectedVersion, nil),
			ok:       true,
			want:     testInjectedVersion,
		},
		{
			name:     "injected sentinel falls back to vcs revision",
			injected: versionUnknown,
			info: testBuildInfo(buildInfoMainVersionDev, map[string]string{
				buildInfoVCSRevisionKey: testRevision,
			}),
			ok:   true,
			want: testRevision,
		},
		{
			name: "main module version is used",
			info: testBuildInfo(testInjectedVersion, nil),
			ok:   true,
			want: testInjectedVersion,
		},
		{
			name: "vcs revision is used when main version is devel",
			info: testBuildInfo(buildInfoMainVersionDev, map[string]string{
				buildInfoVCSRevisionKey: testRevision,
			}),
			ok:   true,
			want: testRevision,
		},
		{
			name: "vcs revision carries dirty suffix when modified",
			info: testBuildInfo(buildInfoMainVersionDev, map[string]string{
				buildInfoVCSModifiedKey: strconv.FormatBool(true),
				buildInfoVCSRevisionKey: testRevision,
			}),
			ok:   true,
			want: testRevisionDirty,
		},
		{
			name: "empty main version uses clean vcs revision",
			info: testBuildInfo("", map[string]string{
				buildInfoVCSModifiedKey: strconv.FormatBool(false),
				buildInfoVCSRevisionKey: "fedcba654321",
			}),
			ok:   true,
			want: "fedcba654321",
		},
		{
			name: "sentinel when build info has no usable version",
			info: testBuildInfo("", nil),
			ok:   true,
			want: versionUnknown,
		},
		{
			name: "sentinel when devel build info has no revision",
			info: testBuildInfo(buildInfoMainVersionDev, map[string]string{
				buildInfoVCSModifiedKey: strconv.FormatBool(true),
			}),
			ok:   true,
			want: versionUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersionFrom(tc.injected, tc.info, tc.ok)
			if got != tc.want {
				t.Fatalf("resolveVersionFrom(%q, info, %t) = %q, want %q", tc.injected, tc.ok, got, tc.want)
			}
		})
	}
}

func FuzzResolveVersion(f *testing.F) {
	f.Add("", "", false)
	f.Add("", testRevision, false)
	f.Add("", testRevision, true)
	f.Add(testInjectedVersion, testRevision, true)

	f.Fuzz(func(t *testing.T, injected string, revision string, modified bool) {
		settings := map[string]string{buildInfoVCSRevisionKey: revision}
		if modified {
			settings[buildInfoVCSModifiedKey] = strconv.FormatBool(true)
		}
		got := resolveVersionFrom(injected, testBuildInfo(buildInfoMainVersionDev, settings), true)
		if got == "" {
			t.Fatal("resolved version is empty")
		}
	})
}
