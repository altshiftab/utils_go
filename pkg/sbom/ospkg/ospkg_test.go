package ospkg

import (
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// expectEmptyError checks that err reports an empty field of the given name.
func expectEmptyError(t *testing.T, err error, field string) {
	t.Helper()
	emptyErr, ok := errors.AsType[*empty_error.Error](err)
	if !ok || emptyErr.Field != field {
		t.Fatalf("expected an empty %q error, got %v", field, err)
	}
}

func TestParseOsRelease(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		data     string
		expected *OsRelease
		empty    bool
	}{
		{
			name:     "alpine",
			data:     "NAME=\"Alpine Linux\"\nID=alpine\nVERSION_ID=3.24.1\nPRETTY_NAME=\"Alpine Linux v3.24\"\nHOME_URL=\"https://alpinelinux.org/\"\n",
			expected: &OsRelease{Id: "alpine", Name: "Alpine Linux", PrettyName: "Alpine Linux v3.24", VersionId: "3.24.1"},
		},
		{
			name:     "debian",
			data:     "PRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\nNAME=\"Debian GNU/Linux\"\nVERSION_ID=\"13\"\nVERSION=\"13 (trixie)\"\nVERSION_CODENAME=trixie\nDEBIAN_VERSION_FULL=13.6\nID=debian\n",
			expected: &OsRelease{Id: "debian", Name: "Debian GNU/Linux", PrettyName: "Debian GNU/Linux 13 (trixie)", VersionId: "13", VersionCodename: "trixie"},
		},
		{
			name:     "ubuntu with id like, comments and single quotes",
			data:     "# generated\nID=ubuntu\nID_LIKE=debian\nVERSION_ID='24.04'\n\nNAME=\"Ubuntu\"\n",
			expected: &OsRelease{Id: "ubuntu", IdLike: "debian", Name: "Ubuntu", VersionId: "24.04"},
		},
		{
			name:     "escapes inside double quotes",
			data:     "ID=test\nPRETTY_NAME=\"Say \\\"hi\\\" for \\$1 \\\\ done\"\n",
			expected: &OsRelease{Id: "test", PrettyName: "Say \"hi\" for $1 \\ done"},
		},
		{
			name:     "no version id (rolling release)",
			data:     "ID=debian\nPRETTY_NAME=\"Debian GNU/Linux trixie/sid\"\n",
			expected: &OsRelease{Id: "debian", PrettyName: "Debian GNU/Linux trixie/sid"},
		},
		{
			name:     "lines without equals are ignored",
			data:     "garbage\nID=x\n",
			expected: &OsRelease{Id: "x"},
		},
		{
			name:  "empty",
			data:  "",
			empty: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			osRelease, err := ParseOsRelease([]byte(testCase.data))
			if testCase.empty {
				expectEmptyError(t, err, "data")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *osRelease != *testCase.expected {
				t.Errorf("expected %+v, got %+v", testCase.expected, osRelease)
			}
		})
	}
}

const apkInstalledFixture = `C:Q18Pz9wvTaBYr2RzvEXE6rYpFiJbI=
P:alpine-baselayout
V:3.7.2-r1
A:x86_64
S:8281
I:6552
T:Alpine base dir structure and init scripts
U:https://gitlab.alpinelinux.org/alpine/aports/-/tree/master/main/alpine-baselayout
L:GPL-2.0-only
o:alpine-baselayout
m:Natanael Copa <ncopa@alpinelinux.org>
t:1774381254
c:60a7585bbab2fa0f762504eb617dbca90216e31f
D:alpine-baselayout-data=3.7.2-r1 /bin/sh
q:1000
F:dev
F:etc
R:motd
Z:Q1SLkS9hBidUbPwwrw+XR0Whv3ww8=

C:Q1abc=
P:ssl_client
V:1.37.0-r31
A:x86_64
S:4900
T:EXternal ssl_client for busybox wget
U:https://busybox.net/
L:GPL-2.0-only
o:busybox
m:Sören Tempel <soeren+alpine@soeren-tempel.net>
t:1774381254
c:abc
D:so:libc.musl-x86_64.so.1 so:libcrypto.so.3 so:libssl.so.3
p:cmd:ssl_client=1.37.0-r31
F:usr
F:usr/bin
R:ssl_client
a:0:0:755
Z:Q1def=

C:Q1ghi=
P:libcrypto3
V:3.5.7-r0
A:x86_64
L:Apache-2.0
o:openssl

`

func TestParseApkInstalled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		data     string
		expected []*ApkPackage
		empty    bool
	}{
		{
			name: "alpine records",
			data: apkInstalledFixture,
			expected: []*ApkPackage{
				{Name: "alpine-baselayout", Version: "3.7.2-r1", Arch: "x86_64", Origin: "alpine-baselayout", License: "GPL-2.0-only"},
				{Name: "ssl_client", Version: "1.37.0-r31", Arch: "x86_64", Origin: "busybox", License: "GPL-2.0-only"},
				{Name: "libcrypto3", Version: "3.5.7-r0", Arch: "x86_64", Origin: "openssl", License: "Apache-2.0"},
			},
		},
		{
			name: "record without trailing blank line and lines without a field letter",
			data: "junk line\nP:musl\nV:1.2.6-r2\nA:aarch64\no:musl",
			expected: []*ApkPackage{
				{Name: "musl", Version: "1.2.6-r2", Arch: "aarch64", Origin: "musl"},
			},
		},
		{
			name:     "record without a name is dropped",
			data:     "V:1.0-r0\nA:x86_64\n\nP:zlib\nV:1.3.2-r0\n",
			expected: []*ApkPackage{{Name: "zlib", Version: "1.3.2-r0"}},
		},
		{
			name:  "empty",
			data:  "",
			empty: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packages, err := ParseApkInstalled([]byte(testCase.data))
			if testCase.empty {
				expectEmptyError(t, err, "data")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(packages) != len(testCase.expected) {
				t.Fatalf("expected %d packages, got %d: %+v", len(testCase.expected), len(packages), packages)
			}
			for i, expected := range testCase.expected {
				if *packages[i] != *expected {
					t.Errorf("package %d: expected %+v, got %+v", i, expected, packages[i])
				}
			}
		})
	}
}

const dpkgStatusFixture = `Package: bsdutils
Essential: yes
Status: install ok installed
Priority: required
Section: utils
Installed-Size: 401
Maintainer: Chris Hofstaedtler <zeha@debian.org>
Architecture: amd64
Multi-Arch: foreign
Source: util-linux (2.41-5)
Version: 1:2.41-5
Pre-Depends: libc6 (>= 2.38), libsystemd0 (>= 254)
Description: basic utilities from 4.4BSD-Lite
 This package contains the bare minimum of BSD utilities needed for a Debian
 system: logger, renice, script, scriptlive, scriptreplay and wall.
 .
 Status: this line is a description continuation, not a field
Homepage: https://github.com/util-linux/util-linux

Package: libgcc-s1
Status: install ok installed
Architecture: amd64
Source: gcc-14
Version: 14.2.0-19

Package: removed-thing
Status: deinstall ok config-files
Architecture: amd64
Version: 1.0-1

Package: half-installed
Status: install ok half-configured
Architecture: amd64
Version: 2.0-1

Package: apt
Status: install ok installed
Architecture: amd64
Version: 3.0.3
`

func TestParseDpkgStatus(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		data     string
		expected []*DpkgPackage
		empty    bool
	}{
		{
			name: "status database keeps installed packages only",
			data: dpkgStatusFixture,
			expected: []*DpkgPackage{
				{Name: "bsdutils", Version: "1:2.41-5", Architecture: "amd64", SourceName: "util-linux", SourceVersion: "2.41-5", Status: "install ok installed"},
				{Name: "libgcc-s1", Version: "14.2.0-19", Architecture: "amd64", SourceName: "gcc-14", Status: "install ok installed"},
				{Name: "apt", Version: "3.0.3", Architecture: "amd64", Status: "install ok installed"},
			},
		},
		{
			name: "distroless status.d entry without status",
			data: "Package: base-files\nVersion: 13.8\nArchitecture: amd64\nMaintainer: Santiago Vila <sanvila@debian.org>\nDescription: Debian base system miscellaneous files\n",
			expected: []*DpkgPackage{
				{Name: "base-files", Version: "13.8", Architecture: "amd64"},
			},
		},
		{
			name: "source with spaces around the version",
			data: "Package: libssl3t64\nStatus: install ok installed\nArchitecture: amd64\nSource: openssl ( 3.5.1-1 )\nVersion: 3.5.1-1\n",
			expected: []*DpkgPackage{
				{Name: "libssl3t64", Version: "3.5.1-1", Architecture: "amd64", SourceName: "openssl", SourceVersion: "3.5.1-1", Status: "install ok installed"},
			},
		},
		{
			name:  "empty",
			data:  "",
			empty: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packages, err := ParseDpkgStatus([]byte(testCase.data))
			if testCase.empty {
				expectEmptyError(t, err, "data")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(packages) != len(testCase.expected) {
				t.Fatalf("expected %d packages, got %d: %+v", len(testCase.expected), len(packages), packages)
			}
			for i, expected := range testCase.expected {
				if *packages[i] != *expected {
					t.Errorf("package %d: expected %+v, got %+v", i, expected, packages[i])
				}
			}
		})
	}
}
