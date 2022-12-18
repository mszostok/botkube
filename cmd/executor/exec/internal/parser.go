package internal

import (
	"bufio"
	"strings"
	"unicode"
)

func ParseTable(in string) ([][]string, []string) {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(in))
	for sc.Scan() {
		txt := sc.Text()
		if len(txt) <= 0 {
			continue
		}
		lines = append(lines, txt)
	}

	// get separators from the first line
	separators := getSeparators(lines[0])

	var out [][]string
	for _, line := range lines {
		out = append(out, getCells(line, separators))
	}

	return out, lines
}

// function takes a line
// and returns a list of separators (positions of left edges of the cells)
func getSeparators(line string) []int {
	var separators []int
	for idx, ch := range line {
		cur := unicode.IsSpace(ch)
		if !cur { // not separator
			continue
		}

		prevv := idx - 1
		if prevv < 0 {
			prevv = 0
		}

		nextx := idx + 1
		if nextx >= len(line) {
			nextx = len(line)
		}

		next := unicode.IsSpace(rune(line[nextx]))
		prev := unicode.IsSpace(rune(line[prevv]))

		if cur && next {
			continue
		}

		if cur && !prev && !next {
			continue
		}
		separators = append(separators, idx)
	}
	return separators
}

// # function takes a line and a list of separators
// # and returns a list of cells (the line divided by the separators)
func getCells(line string, separators []int) []string {
	var res []string
	start := 0
	for _, end := range separators {
		if end > len(line) {
			end = len(line)
		}
		cell := strings.TrimSpace(line[start:end])
		start = end
		res = append(res, cell)
	}
	return res
}
