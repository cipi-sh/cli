package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

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
	w       *tabwriter.Writer
	headers []string
}

func NewTable(headers ...string) *Table {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 3, ' ', 0)
	t := &Table{w: w, headers: headers}
	t.printHeader()
	return t
}

func (t *Table) printHeader() {
	colored := make([]string, len(t.headers))
	for i, h := range t.headers {
		colored[i] = Dim.Sprint(h)
	}
	fmt.Fprintf(t.w, "  %s\n", strings.Join(colored, "\t"))

	seps := make([]string, len(t.headers))
	for i, h := range t.headers {
		seps[i] = Dim.Sprint(strings.Repeat("─", len(h)))
	}
	fmt.Fprintf(t.w, "  %s\n", strings.Join(seps, "\t"))
}

func (t *Table) Row(values ...string) {
	fmt.Fprintf(t.w, "  %s\n", strings.Join(values, "\t"))
}

func (t *Table) Flush() {
	t.w.Flush()
	fmt.Println()
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
