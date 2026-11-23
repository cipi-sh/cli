package cmd

import "testing"

func TestMetSpark(t *testing.T) {
	if got := metSpark([]float64{0, 7, 14}); got != "▁▄█" {
		t.Fatalf("spark = %q", got)
	}
	if got := metSpark([]float64{5, 5}); got != "▁▁" {
		t.Fatalf("flat spark = %q", got)
	}
	if got := metSpark(nil); got != "" {
		t.Fatalf("empty spark = %q", got)
	}
}

func TestMetFormatting(t *testing.T) {
	if got := metRate(1536.0); got != "1.5 KB/s" {
		t.Fatalf("rate = %q", got)
	}
	if got := metNum(nil, 1); got != "—" {
		t.Fatalf("nil num = %q", got)
	}
	if got := metNum(12.345, 1); got != "12.3" {
		t.Fatalf("num = %q", got)
	}
	if got := metFloats([]interface{}{1.0, nil, "2.5"}); len(got) != 2 || got[1] != 2.5 {
		t.Fatalf("floats = %v", got)
	}
	if metOnOff(true) != "on" || metOnOff(nil) != "off" {
		t.Fatal("onOff")
	}
}
