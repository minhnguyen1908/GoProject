package logger

import (
	"io"
	"log"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Setup initializes the logging system.
// It configures the standard "log" package to write to both Terminal and File.
func Setup(filename string) {
	// 1. Configure the Rotating File
	logFile := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    10,   // Megabytes
		MaxBackups: 5,    // Files
		MaxAge:     28,   // Days
		Compress:   true, // .gz
	}

	// 2. Combine Outpusts (Terminal + File)
	// We use io.MultiWriter so we can see it live AND save it for later.
	mw := io.MultiWriter(os.Stdout, logFile)

	// 3. Set the Global Logger
	log.SetOutput(mw)

	// 4. Add Context (Flags)
	// LstdFlags = Date + Time
	// Lshortfile = file.go:line_number (Crucial for tracing!)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println("✅ Log System Initialized: Writing to console and ", filename)
}
