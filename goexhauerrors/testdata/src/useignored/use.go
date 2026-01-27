package useignored

import "ignored"

// UseIgnoredPackage demonstrates that when "ignored" package is in ignorePackages,
// no error check warnings should be generated.
func UseIgnoredPackage() {
	// These should NOT generate warnings because "ignored" package is in ignorePackages
	err := ignored.GetIgnoredError()
	if err != nil {
		println(err.Error())
	}

	err2 := ignored.GetIgnoredCustomError()
	if err2 != nil {
		println(err2.Error())
	}
}
