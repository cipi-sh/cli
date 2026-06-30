package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
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
	HiCyan    = color.New(color.FgHiCyan, color.Bold)
	HiGreen   = color.New(color.FgHiGreen, color.Bold)
	HiYellow  = color.New(color.FgHiYellow, color.Bold)
	HiMagenta = color.New(color.FgHiMagenta, color.Bold)
	HiBlue    = color.New(color.FgHiBlue, color.Bold)
)

var (
	JSONOutput bool
	ansiRE     = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

const indent = "  "

func Success(msg string, args ...interface{}) {
	label := Green.Sprint("✓")
	fmt.Printf("%s%s %s\n", indent, label, fmt.Sprintf(msg, args...))
}

func Error(msg string, args ...interface{}) {
	label := Red.Sprint("✗")
	Red.Fprintf(os.Stderr, "%s%s %s\n", indent, label, fmt.Sprintf(msg, args...))
}

func Warn(msg string, args ...interface{}) {
	label := Yellow.Sprint("!")
	fmt.Printf("%s%s %s\n", indent, label, fmt.Sprintf(msg, args...))
}

func Info(msg string, args ...interface{}) {
	label := HiCyan.Sprint("→")
	fmt.Printf("%s%s %s\n", indent, label, fmt.Sprintf(msg, args...))
}

func PrintJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func Header(title string) {
	fmt.Println()
	HiCyan.Printf("%s◆ %s\n", indent, Bold.Sprint(title))
	lineLen := min(utf8.RuneCountInString(title)+4, 52)
	Dim.Printf("%s  %s\n", indent, strings.Repeat("─", lineLen))
}

func Divider() {
	Dim.Printf("%s%s\n", indent, strings.Repeat("·", 40))
}

func Footer(format string, args ...interface{}) {
	Dim.Printf("%s%s\n", indent, fmt.Sprintf(format, args...))
}

const bannerInnerWidth = 40

func Banner() {
	fmt.Println()
	HiCyan.Println(indent + "╭──────────────────────────────────────────╮")
	lines := []string{
		"     ██████╗██╗██████╗ ██╗",
		"    ██╔════╝██║██╔══██╗██║",
		"    ██║     ██║██████╔╝██║",
		"    ██║     ██║██╔═══╝ ██║",
		"    ╚██████╗██║██║     ██║",
		"     ╚═════╝╚═╝╚═╝     ╚═╝",
	}
	for _, line := range lines {
		printBannerLine(line)
	}
	HiCyan.Println(indent + "├──────────────────────────────────────────┤")
	Dim.Printf("%s│%s│\n", indent, centerText("CLI for Cipi Server Panel", bannerInnerWidth))
	HiCyan.Println(indent + "╰──────────────────────────────────────────╯")
	fmt.Println()
}

func printBannerLine(content string) {
	HiCyan.Print(indent + "│")
	HiBlue.Print(padRight(content, bannerInnerWidth))
	HiCyan.Println("│")
}

func centerText(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return " " + s + " "
	}
	pad := width - n
	left := pad / 2
	right := pad - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func Welcome() {
	Banner()
	Dim.Println(indent + "Quick start")
	fmt.Println()
	ListItem("cipi-cli configure --profile prod", "Set up your API endpoint and token")
	ListItem("cipi-cli prod apps list", "Manage apps on a specific server profile")
	fmt.Println()
	Dim.Printf("%sRun %s for the full command list.\n\n", indent, Cyan.Sprint("cipi-cli --help"))
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
	if len(t.headers) == 0 {
		return
	}

	colWidths := make([]int, len(t.headers))
	for i, h := range t.headers {
		colWidths[i] = visibleWidth(h)
	}
	for _, row := range t.rows {
		for i, v := range row {
			if i < len(colWidths) {
				if w := visibleWidth(v); w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	var b strings.Builder
	b.WriteString(indent)
	b.WriteString("┌")
	for i, w := range colWidths {
		if i > 0 {
			b.WriteString("─┬")
		}
		b.WriteString(strings.Repeat("─", w+2))
	}
	b.WriteString("─┐\n")

	b.WriteString(indent)
	b.WriteString("│")
	for i, h := range t.headers {
		if i > 0 {
			b.WriteString("│")
		}
		b.WriteString(" ")
		b.WriteString(Dim.Sprint(padRight(h, colWidths[i])))
		b.WriteString(" ")
	}
	b.WriteString("│\n")

	b.WriteString(indent)
	b.WriteString("├")
	for i, w := range colWidths {
		if i > 0 {
			b.WriteString("─┼")
		}
		b.WriteString(strings.Repeat("─", w+2))
	}
	b.WriteString("─┤\n")

	for rowIdx, row := range t.rows {
		b.WriteString(indent)
		b.WriteString("│")
		for i := 0; i < len(t.headers); i++ {
			if i > 0 {
				b.WriteString("│")
			}
			v := ""
			if i < len(row) {
				v = row[i]
			}
			if rowIdx%2 == 1 && !strings.Contains(v, "\x1b[") {
				v = Dim.Sprint(padRightVisible(v, colWidths[i]))
			} else {
				v = padRightVisible(v, colWidths[i])
			}
			b.WriteString(" ")
			b.WriteString(v)
			b.WriteString(" ")
		}
		b.WriteString("│\n")
	}

	b.WriteString(indent)
	b.WriteString("└")
	for i, w := range colWidths {
		if i > 0 {
			b.WriteString("─┴")
		}
		b.WriteString(strings.Repeat("─", w+2))
	}
	b.WriteString("─┘\n")

	fmt.Print(b.String())
	fmt.Println()
}

func KeyValue(w io.Writer, key, value string) {
	if w == nil {
		w = os.Stdout
	}
	Dim.Fprintf(w, "%s%-14s", indent, key)
	Dim.Fprint(w, " ")
	fmt.Fprintf(w, "%s\n", value)
}

func KeyValueDots(w io.Writer, key, value string) {
	if w == nil {
		w = os.Stdout
	}
	const keyWidth = 14
	dots := 28 - keyWidth
	if dots < 2 {
		dots = 2
	}
	Dim.Fprintf(w, "%s%-14s %s ", indent, key, strings.Repeat("·", dots))
	fmt.Fprintf(w, "%s\n", value)
}

func ListItem(label, description string) {
	HiMagenta.Printf("%s• %s\n", indent, label)
	if description != "" {
		Dim.Printf("%s  %s\n", indent, description)
	}
}

func Confirm(prompt string) bool {
	fmt.Println()
	Yellow.Printf("%s? %s ", indent, prompt)
	Dim.Print("[y/N] ")
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func ReadInput(prompt string) string {
	Cyan.Printf("%s▸ %s ", indent, prompt)
	var input string
	fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

func ReadSecret(prompt string) string {
	return ReadInput(prompt)
}

func StatusYesNo(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1":
		return HiGreen.Sprint("yes")
	case "no", "false", "0", "—", "":
		return Dim.Sprint("no")
	default:
		return value
	}
}

func StatusSuspended(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1":
		return Yellow.Sprint("suspended")
	default:
		return HiGreen.Sprint("active")
	}
}

func StatusJob(status string) string {
	switch strings.ToLower(status) {
	case "completed", "success", "finished":
		return Green.Sprint(status)
	case "failed", "error":
		return Red.Sprint(status)
	case "pending", "processing", "running":
		return Yellow.Sprint(status)
	default:
		return status
	}
}

func KindBadge(kind string) string {
	switch strings.ToLower(kind) {
	case "primary":
		return HiCyan.Sprint(kind)
	case "alias":
		return HiMagenta.Sprint(kind)
	default:
		return kind
	}
}

func ProfileName(name string, isDefault bool) string {
	if isDefault {
		return fmt.Sprintf("%s %s", Bold.Sprint(name), Dim.Sprint("(default)"))
	}
	return Bold.Sprint(name)
}

func visibleWidth(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func padRightVisible(s string, width int) string {
	n := visibleWidth(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
