package util

import "fmt"

type SemVer struct {
	Major byte
	Minor byte
	Patch byte
}

var ServerVersion = SemVer{Major: 0, Minor: 5, Patch: 0}

func (s SemVer) String() string {
	return fmt.Sprintf("v%d.%d.%d", s.Major, s.Minor, s.Patch)
}

func (s SemVer) Eq(other SemVer) bool {
	return s.Major == other.Major && s.Minor == other.Minor && s.Patch == other.Patch
}

func (s SemVer) Compatible(other SemVer) bool {
	return s.Major == other.Major && s.Minor >= other.Minor
}
