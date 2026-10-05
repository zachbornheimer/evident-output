package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// errFrameTooLarge is a message over maxFrameBytes. readMCPMessage has
// already skipped past it, so the server answers it and keeps reading.
var errFrameTooLarge = fmt.Errorf("message exceeds %d bytes", maxFrameBytes)

// readMCPMessage reads one JSON-RPC message from r.
// Supports NDJSON (spec) and LSP-style Content-Length frames (some clients).
func readMCPMessage(r *bufio.Reader) ([]byte, framingMode, error) {
	if err := skipBlankLines(r); err != nil {
		return nil, frameNDJSON, err
	}
	peek, err := r.Peek(1)
	if err != nil {
		return nil, frameNDJSON, err
	}
	// Content-Length header (case-insensitive) — used by some MCP client SDKs.
	if peek[0] == 'C' || peek[0] == 'c' {
		return readContentLengthFrame(r)
	}
	return readNDJSONFrame(r)
}

// skipBlankLines consumes leading CR/LF, leaving r at a message's first byte.
func skipBlankLines(r *bufio.Reader) error {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b != '\n' && b != '\r' {
			return r.UnreadByte()
		}
	}
}

// readContentLengthFrame reads one Content-Length framed body. A line that
// starts with C but is no such header is returned as a (broken) NDJSON line.
func readContentLengthFrame(r *bufio.Reader) ([]byte, framingMode, error) {
	headerLine, err := r.ReadString('\n')
	if err != nil {
		return nil, frameContentLength, err
	}
	headerLine = strings.TrimRight(headerLine, "\r\n")
	name, value, _ := strings.Cut(headerLine, ":")
	if !strings.EqualFold(strings.TrimSpace(name), "content-length") {
		return []byte(headerLine), frameNDJSON, nil
	}
	nStr := strings.TrimSpace(value)
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 0 {
		return nil, frameContentLength, fmt.Errorf("invalid Content-Length %q", nStr)
	}
	// Consume optional additional headers until blank line.
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, frameContentLength, err
		}
		if line == "\n" || line == "\r\n" {
			break
		}
	}
	if n > maxFrameBytes {
		// Discard the body so the next message can resync.
		if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
			return nil, frameContentLength, err
		}
		return nil, frameContentLength, fmt.Errorf("%w: Content-Length %d", errFrameTooLarge, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, frameContentLength, err
	}
	return body, frameContentLength, nil
}

// readNDJSONFrame reads one newline-terminated JSON object, at most
// maxFrameBytes long. A final line without a newline is still a message.
func readNDJSONFrame(r *bufio.Reader) ([]byte, framingMode, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		content := chunk
		if err == nil {
			content = chunk[:len(chunk)-1]
		}
		if len(line)+len(content) > maxFrameBytes {
			if errors.Is(err, bufio.ErrBufferFull) {
				discardLine(r)
			}
			return nil, frameNDJSON, fmt.Errorf("%w: ndjson frame", errFrameTooLarge)
		}
		line = append(line, content...)
		switch {
		case err == nil:
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case len(line) == 0:
			return nil, frameNDJSON, err
		}
		return bytes.TrimRight(line, "\r"), frameNDJSON, nil
	}
}

// discardLine skips the rest of the current line, so the next message can
// resync after an oversize one.
func discardLine(r *bufio.Reader) {
	for {
		if _, err := r.ReadSlice('\n'); !errors.Is(err, bufio.ErrBufferFull) {
			return
		}
	}
}
