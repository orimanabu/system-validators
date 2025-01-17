//go:build linux
// +build linux

/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package system

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
)

func TestExtractUpstreamVersion(t *testing.T) {
	for _, test := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		},
		{
			input:    "1.0.6",
			expected: "1.0.6",
		},
		{
			input:    "1:1.0.6",
			expected: "1.0.6",
		},
		{
			input:    "1.0.6-2ubuntu2.1",
			expected: "1.0.6",
		},
		{
			input:    "1:1.0.6-2ubuntu2.1",
			expected: "1.0.6",
		},
	} {
		t.Run(fmt.Sprintf("input:%s,expected:%s", test.input, test.expected), func(t *testing.T) {
			got := extractUpstreamVersion(test.input)
			if test.expected != got {
				t.Errorf("extractUpstreamVersion(%q) = %q, want %q", test.input, got, test.expected)
			}
		})
	}
}

func TestToSemVer(t *testing.T) {
	for _, test := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		},
		{
			input:    "1.2.3",
			expected: "1.2.3",
		},
		{
			input:    "1.8.19p1",
			expected: "1.8.19",
		},
		{
			input:    "1.8.19.p1",
			expected: "1.8.19",
		},
		{
			input:    "p1",
			expected: "",
		},
		{
			input:    "1.18",
			expected: "1.18.0",
		},
		{
			input:    "481",
			expected: "481.0.0",
		},
		{
			input:    "2.0.10.4",
			expected: "2.0.10",
		},
		{
			input:    "03",
			expected: "3.0.0",
		},
		{
			input:    "2.02",
			expected: "2.2.0",
		},
		{
			input:    "8.0.0095",
			expected: "8.0.95",
		},
	} {
		t.Run(fmt.Sprintf("input:%s,expected:%s", test.input, test.expected), func(t *testing.T) {
			got := toSemVer(test.input)
			if test.expected != got {
				t.Errorf("toSemVer(%q) = %q, want %q", test.input, got, test.expected)
			}
		})
	}
}

func TestToSemVerRange(t *testing.T) {
	for _, test := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		},
		{
			input:    ">=1.0.0",
			expected: ">=1.0.0",
		},
		{
			input:    ">=1.0",
			expected: ">=1.0.x",
		},
		{
			input:    ">=1",
			expected: ">=1.x",
		},
		{
			input:    ">=1 || !2.3",
			expected: ">=1.x || !2.3.x",
		},
		{
			input:    ">1 || >3.1.0 !4.2",
			expected: ">1.x || >3.1.0 !4.2.x",
		},
	} {
		t.Run(fmt.Sprintf("input:%s,expected:%s", test.input, test.expected), func(t *testing.T) {
			got := toSemVerRange(test.input)
			if test.expected != got {
				t.Errorf("toSemVerRange(%q) = %q, want %q", test.input, got, test.expected)
			}
		})
	}
}

// testPackageManager implements the packageManager interface.
type testPackageManager struct {
	packageVersions map[string]string
}

func (m testPackageManager) getPackageVersion(packageName string) (string, error) {
	if v, ok := m.packageVersions[packageName]; ok {
		return v, nil
	}
	return "", errors.Errorf("package %q does not exist", packageName)
}

func TestValidatePackageVersion(t *testing.T) {
	testKernelRelease := "test-kernel-release"
	manager := testPackageManager{
		packageVersions: map[string]string{
			"foo":                      "1.0.0",
			"bar":                      "2.1.0",
			"bar-" + testKernelRelease: "3.0.0",
		},
	}
	v := &packageValidator{
		reporter:      DefaultReporter,
		kernelRelease: testKernelRelease,
	}
	for _, test := range []struct {
		desc  string
		specs []PackageSpec
		errs  []error
	}{
		{
			desc: "all packages meet the spec",
			specs: []PackageSpec{
				{Name: "foo", VersionRange: ">=1.0"},
				{Name: "bar", VersionRange: ">=2.0 <= 3.0"},
			},
		},
		{
			desc: "package version does not meet the spec",
			specs: []PackageSpec{
				{Name: "foo", VersionRange: ">=1.0"},
				{Name: "bar", VersionRange: ">=3.0"},
			},
			errs: []error{
				errors.New("package \"bar 2.1.0\" does not meet the spec \"bar (>=3.0)\""),
			},
		},
		{
			desc: "package not installed",
			specs: []PackageSpec{
				{Name: "baz"},
			},
			errs: []error{
				errors.New("package \"baz\" does not exist"),
			},
		},
		{
			desc: "use variable in package name",
			specs: []PackageSpec{
				{Name: "bar-${KERNEL_RELEASE}", VersionRange: ">=3.0"},
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			_, errs := v.validate(test.specs, manager)
			if len(test.errs) == 0 && len(errs) > 0 {
				t.Errorf("%s: v.validate(): err = %s", test.desc, errs)
			}
			if len(test.errs) > 0 {
				if len(errs) == 0 {
					t.Errorf("%s: v.validate() is expected to fail.", test.desc)
				} else if len(errs) != len(test.errs) {
					t.Errorf("%s: v.validate(): errs = %q, want = %q", test.desc, errs, test.errs)
				} else {
					for i, v := range test.errs {
						if v.Error() != errs[i].Error() {
							t.Errorf("%s: v.validate(): errs = %q, want = %q", test.desc, errs, test.errs)
						}
					}
				}
			}
		})
	}
}

func TestApplyPackageOverride(t *testing.T) {
	for _, test := range []struct {
		name      string
		overrides []PackageSpecOverride
		osDistro  string
		specs     []PackageSpec
		expected  []PackageSpec
	}{
		{
			name:     "foo>=1.0",
			specs:    []PackageSpec{{Name: "foo", VersionRange: ">=1.0"}},
			expected: []PackageSpec{{Name: "foo", VersionRange: ">=1.0"}},
		},
		{
			name:     "ubuntu:foo>=1.0/bar>=2.0",
			osDistro: "ubuntu",
			overrides: []PackageSpecOverride{
				{
					OSDistro:     "ubuntu",
					Subtractions: []PackageSpec{{Name: "foo"}},
					Additions:    []PackageSpec{{Name: "bar", VersionRange: ">=2.0"}},
				},
			},
			specs:    []PackageSpec{{Name: "foo", VersionRange: ">=1.0"}},
			expected: []PackageSpec{{Name: "bar", VersionRange: ">=2.0"}},
		},
		{
			name:     "ubuntu:foo>=1.0/debian:foo",
			osDistro: "ubuntu",
			overrides: []PackageSpecOverride{
				{
					OSDistro:     "debian",
					Subtractions: []PackageSpec{{Name: "foo"}},
				},
			},
			specs:    []PackageSpec{{Name: "foo", VersionRange: ">=1.0"}},
			expected: []PackageSpec{{Name: "foo", VersionRange: ">=1.0"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := applyPackageSpecOverride(test.specs, test.overrides, test.osDistro)
			if !reflect.DeepEqual(test.expected, got) {
				t.Errorf("applyPackageSpecOverride(%+v, %+v, %s) = %+v, want = %+v", test.specs, test.overrides, test.osDistro, got, test.expected)
			}
		})
	}
}
