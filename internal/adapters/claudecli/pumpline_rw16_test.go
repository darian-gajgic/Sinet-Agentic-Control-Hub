package claudecli

// P3-RW-16 R5 unit tests (executor-added): the length-safe line reader that
// replaced bufio.Scanner in the pump. The e2e test proves the wedge is gone;
// these pin the reader's own edges — the ones a Scanner used to handle and
// that the tail of a stream depends on.

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func readAll(t *testing.T, in string, limit int) (lines []string, skips []int) {
	t.Helper()
	br := bufio.NewReaderSize(strings.NewReader(in), 16)
	for {
		line, n, skipped, err := readCappedLine(br, limit)
		if skipped {
			skips = append(skips, n)
		} else if len(line) > 0 {
			lines = append(lines, string(line))
		}
		if err != nil {
			if err != io.EOF {
				t.Fatalf("read: %v", err)
			}
			break
		}
	}
	return lines, skips
}

func TestRW16ReadCappedLineKeepsScannerSemantics(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  []string
	}{
		{"plain lines", "a\nbb\nccc\n", 1 << 20, []string{"a", "bb", "ccc"}},
		{"final line without a newline", "a\nlast", 1 << 20, []string{"a", "last"}},
		{"CRLF terminators are stripped", "a\r\nb\r\n", 1 << 20, []string{"a", "b"}},
		{"blank lines carry nothing", "a\n\n\nb\n", 1 << 20, []string{"a", "b"}},
		{"a line longer than the read buffer still arrives whole",
			strings.Repeat("x", 200) + "\n", 1 << 20, []string{strings.Repeat("x", 200)}},
		{"empty stream", "", 1 << 20, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, skips := readAll(t, tc.in, tc.limit)
			if len(skips) != 0 {
				t.Fatalf("unexpected skips %v", skips)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("lines %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRW16ReadCappedLineSkipsAndKeepsReading is R5's heart at unit scale:
// the oversized line is discarded through its newline and everything after
// it — the tail the terminal envelope lives in — still arrives.
func TestRW16ReadCappedLineSkipsAndKeepsReading(t *testing.T) {
	big := strings.Repeat("x", 500)
	in := "head\n" + big + "\nmiddle\n" + big + "\ntail\n"
	got, skips := readAll(t, in, 100)
	if strings.Join(got, "|") != "head|middle|tail" {
		t.Fatalf("lines %q, want head|middle|tail", got)
	}
	if len(skips) != 2 {
		t.Fatalf("skips %v, want two", skips)
	}
	for _, n := range skips {
		if n != len(big)+1 { // the reported length includes the newline
			t.Fatalf("skip reported %d bytes, want %d", n, len(big)+1)
		}
	}
}

// An oversized FINAL line (no trailing newline) must end the read cleanly
// rather than surface as a partial line.
func TestRW16ReadCappedLineSkipsAnUnterminatedOversizedTail(t *testing.T) {
	got, skips := readAll(t, "head\n"+strings.Repeat("x", 500), 100)
	if strings.Join(got, "|") != "head" {
		t.Fatalf("lines %q, want head", got)
	}
	if len(skips) != 1 || skips[0] != 500 {
		t.Fatalf("skips %v, want one of 500 bytes", skips)
	}
}
