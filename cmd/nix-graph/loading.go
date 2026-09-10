package main

import (
	"fmt"
	"time"
)

const spinnerFrame = 100 * time.Millisecond

// loadGraph runs Load in the background and drives a spinner until it
// finishes.
func loadGraph(root string) (*Graph, error) {
	done := make(chan struct{})
	var g *Graph
	var err error
	go func() {
		defer close(done)
		g, err = Load(root)
	}()
	spinner(done, "building dependency tree of "+root)
	return g, err
}

func spinner(done <-chan struct{}, msg string) {
	start := time.Now()
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	for i := 0; ; i = (i + 1) % len(frames) {
		w, h := termSizeOr(80, 24)
		elapsed := fmt.Sprintf("%.1fs", time.Since(start).Seconds())
		line, lw := loadingLine(frames[i], elapsed, msg, w)
		row, col := centerPos(w, h, lw)
		fmt.Printf("\x1b[%d;%dH\x1b[2K%s", row, col, line)
		select {
		case <-done:
			fmt.Printf("\x1b[%d;%dH\x1b[2K", row, col)
			return
		case <-time.After(spinnerFrame):
		}
	}
}

func loadingLine(frame, elapsed, msg string, w int) (string, int) {
	msgW := w - runeLen(frame) - runeLen(elapsed) - 2
	if msgW < 0 {
		return cyan + frame + reset, runeLen(frame)
	}
	if runeLen(msg) > msgW {
		msg = truncate(msg, msgW)
	}
	line := cyan + frame + reset + " " + dim + elapsed + reset + " " + white + msg + reset
	return line, runeLen(frame) + 1 + runeLen(elapsed) + 1 + runeLen(msg)
}

func centerPos(w, h, lw int) (row, col int) {
	row = h / 2
	if row < 1 {
		row = 1
	}
	col = (w-lw)/2 + 1
	if col < 1 {
		col = 1
	}
	return row, col
}
