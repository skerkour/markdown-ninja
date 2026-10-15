package list

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"markdown.ninja/stdx/set"
)

type StringList []string

func LoadStringList(rawData []byte) (list StringList, err error) {
	elements := set.New[string]()

	listScanner := bufio.NewScanner(bytes.NewReader(rawData))
	for listScanner.Scan() {
		line := strings.TrimSpace(listScanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		elements.Insert(line)
	}
	if err = listScanner.Err(); err != nil {
		err = fmt.Errorf("list: error scanning input: %w", err)
		return
	}

	list = elements.ToSlice()

	return
}

func NewStringList(list []string) StringList {
	if list == nil {
		list = make([]string, 0)
	}
	return list
}

// return true if any element of the list contains the string
func (list StringList) AnyElementContains(input string) bool {
	for _, listElement := range list {
		if strings.Contains(input, listElement) {
			return true
		}
	}

	return false
}

// listEndsWithString returns true if input ends with any element of list and false otherwise
func (list StringList) EndsWith(input string) bool {
	for _, listElement := range list {
		if strings.HasSuffix(input, listElement) {
			return true
		}
	}

	return false
}
