package kapt

// blow up on errors that can only happen when kapt itself is broken
func check(err error) {
	if err != nil {
		panic(err)
	}
}
