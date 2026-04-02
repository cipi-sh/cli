package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
)

var (
	Bold      = color.New(color.Bold)
	Cyan      = color.New(color.FgCyan, color.Bold)
	Green     = color.New(color.FgGreen, color.Bold)
	Red       = color.New(color.FgRed, color.Bold)
	Yellow    = color.New(color.FgYellow, color.Bold)
	Dim       = color.New(color.Faint)
	White     = color.New(color.FgWhite)
	HiCyan    = color.New(color.FgHiCyan)
	HiGreen   = color.New(color.FgHiGreen)
	HiYellow  = color.New(color.FgHiYellow)
	HiMagenta = color.New(color.FgHiMagenta)
)

var JSONOutput bool

func Success(msg string, args ...interface{}) {
	Green.Printf("  ✓ "+msg+"\n", args...)
}

func Error(msg string, args ...interface{}) {
	Red.Fprintf(os.Stderr, "  ✗ "+msg+"\n", args...)
}

func Warn(msg string, args ...interface{}) {
	Yellow.Printf("  ! "+msg+"\n", args...)
}

func Info(msg string, args ...interface{}) {
	Cyan.Printf("  → "+msg+"\n", args...)
}

func PrintJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func Header(title string) {
	fmt.Println()
	Bold.Printf("  %s\n", title)
	Dim.Printf("  %s\n", strings.Repeat("─", len(title)+2))
}

type Table struct {
	headers []string
	rows    [][]string
}

func NewTable(headers ...string) *Table {
	return &Table{headers: headers}
}

func (t *Table) Row(values ...string) {
	t.rows = append(t.rows, values)
}

func (t *Table) Flush() {
	colWidths := make([]int, len(t.headers))
	for i, h := range t.headers {
		colWidths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range t.rows {
		for i, v := range row {
			if i < len(colWidths) {
				if w := utf8.RuneCountInString(v); w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	gap := "   "

	hdr := make([]string, len(t.headers))
	for i, h := range t.headers {
		hdr[i] = Dim.Sprint(padRight(h, colWidths[i]))
	}
	fmt.Printf("  %s\n", strings.Join(hdr, gap))

	sep := make([]string, len(t.headers))
	for i, h := range t.headers {
		sep[i] = Dim.Sprint(padRight(strings.Repeat("─", utf8.RuneCountInString(h)), colWidths[i]))
	}
	fmt.Printf("  %s\n", strings.Join(sep, gap))

	for _, row := range t.rows {
		cells := make([]string, len(t.headers))
		for i := 0; i < len(t.headers); i++ {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			cells[i] = padRight(v, colWidths[i])
		}
		fmt.Printf("  %s\n", strings.Join(cells, gap))
	}
	fmt.Println()
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func KeyValue(w io.Writer, key, value string) {
	if w == nil {
		w = os.Stdout
	}
	Dim.Fprintf(w, "  %-16s", key)
	fmt.Fprintf(w, "%s\n", value)
}

func Confirm(prompt string) bool {
	Yellow.Printf("  ? %s [y/N] ", prompt)
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func ReadInput(prompt string) string {
	Cyan.Printf("  %s: ", prompt)
	var input string
	fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

func ReadSecret(prompt string) string {
	Cyan.Printf("  %s: ", prompt)
	var input string
	fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

func Banner() {
	HiCyan.Println()
	HiCyan.Println("     ██████╗██╗██████╗ ██╗")
	HiCyan.Println("    ██╔════╝██║██╔══██╗██║")
	HiCyan.Println("    ██║     ██║██████╔╝██║")
	HiCyan.Println("    ██║     ██║██╔═══╝ ██║")
	HiCyan.Println("    ╚██████╗██║██║     ██║")
	HiCyan.Println("     ╚═════╝╚═╝╚═╝     ╚═╝")
	Dim.Println("     CLI for Cipi Server Panel")
	fmt.Println()
}
