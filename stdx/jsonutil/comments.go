package jsonutil

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// StripComments removes full-line // comments from JSON input.
// Lines whose first non-whitespace characters are "//" are dropped.
// Inline comments (after JSON content) and block comments (/* */) are not handled.
func StripComments(input []byte) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, len(input)))
	scanner := bufio.NewScanner(bytes.NewReader(input))

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning input: %w", err)
	}

	return buf.Bytes(), nil
}
