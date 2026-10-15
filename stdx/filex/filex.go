package filex

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Exists returns true if the file exists or false otherwise
func Exists(file string) (ret bool, err error) {
	_, err = os.Stat(file)

	if err == nil {
		ret = true
	} else if errors.Is(err, os.ErrNotExist) {
		ret = false
		err = nil
	}

	return
}

// Copy the src file to dst
func Copy(dst, src string) (err error) {
	sourceFileInfo, err := os.Stat(src)
	if err != nil {
		return
	}

	if !sourceFileInfo.Mode().IsRegular() {
		err = fmt.Errorf("%s is not a regular file", src)
		return
	}

	source, err := os.Open(src)
	if err != nil {
		return
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return
}
