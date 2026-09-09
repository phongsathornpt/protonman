package readfile

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin/support"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func readTextBytes(ctx context.Context, file *os.File, fileInfo os.FileInfo, input readFileInput, call tool.Call) (tool.Result, error) {
	size := fileInfo.Size()
	continuation, err := support.ContinuationToken("read", struct {
		Path string `json:"path"`
	}{Path: input.Path}, support.FileSnapshot(fileInfo))
	if err != nil {
		_ = file.Close()
		return tool.Result{}, err
	}
	if input.Continuation != "" && input.Continuation != continuation {
		_ = file.Close()
		return tool.Result{}, support.StalePaginationError("read", "read continuation is stale; restart from offset 0", call.Arguments)
	}
	if input.Offset > size {
		input.Offset = size
	}
	if input.Offset > 0 && input.Offset < size {
		var boundary [1]byte
		if _, err := file.ReadAt(boundary[:], input.Offset); err != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("inspect %q at byte %d: %w", input.Path, input.Offset, err)
		}
		if !utf8.RuneStart(boundary[0]) {
			_ = file.Close()
			return tool.Result{}, tool.NewToolError(
				tool.ErrorCodeInvalidArguments,
				fmt.Sprintf("read offset %d splits a UTF-8 code point; use next_offset from the previous page", input.Offset),
			)
		}
	}
	if input.Offset > 0 {
		if _, err := file.Seek(input.Offset, io.SeekStart); err != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("seek %q to byte %d: %w", input.Path, input.Offset, err)
		}
	}

	remaining := size - input.Offset
	truncated := remaining > int64(input.Limit)
	readBytes := remaining
	if truncated {
		readBytes = int64(input.Limit + utf8.UTFMax - 1)
		if readBytes > remaining {
			readBytes = remaining
		}
	}
	contents := make([]byte, int(readBytes))
	_, readErr := io.ReadFull(file, contents)
	closeErr := file.Close()
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return tool.Result{}, fmt.Errorf("read %q: %w", input.Path, readErr)
	}
	if closeErr != nil {
		return tool.Result{}, fmt.Errorf("close %q: %w", input.Path, closeErr)
	}

	if truncated {
		cut, err := utf8PageCut(contents, input.Limit)
		if err != nil {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not valid UTF-8 near byte %d", input.Path, input.Offset))
		}
		contents = contents[:cut]
	} else if !utf8.Valid(contents) {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not valid UTF-8", input.Path))
	}
	if bytes.IndexByte(contents, 0) >= 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q contains NUL bytes and is not a text artifact; use metadata view for binary files", input.Path))
	}

	var nextOffset *int64
	if truncated {
		next := input.Offset + int64(len(contents))
		nextOffset = &next
	}
	output := string(contents)
	if truncated {
		output += fmt.Sprintf("\n[output truncated; continue with offset=%d]", *nextOffset)
	}

	var contentSHA256 string
	if input.Offset == 0 && !truncated {
		digest := sha256.Sum256(contents)
		contentSHA256 = fmt.Sprintf("%x", digest[:])
	}
	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     output,
		SHA256:     contentSHA256,
		Truncated:  truncated,
		NextOffset: nextOffset,
		Continuation: func() string {
			if truncated {
				return continuation
			}
			return ""
		}(),
		Pagination: support.PaginationState(truncated, "offset", nextOffset, nil, continuation),
	}, nil
}
func readFileLines(ctx context.Context, file *os.File, input readFileInput, call tool.Call) (tool.Result, error) {
	return readFileLinesBounded(ctx, file, input, call, runtimepolicy.ReadFileMaxLineScanBytes)
}

func readFileLinesBounded(ctx context.Context, file *os.File, input readFileInput, call tool.Call, maxScanBytes int64) (tool.Result, error) {
	start := input.StartLine
	if start <= 0 {
		start = 1
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), MaxReadFileBytes+utf8.UTFMax)
	scanner.Split(scanLinesKeepEnd)

	var output strings.Builder
	lineNumber := 0
	var scannedBytes int64
	truncated := false
	var nextLine *int
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("read %q by line: %w", input.Path, err)
		}
		line := scanner.Bytes()
		scannedBytes += int64(len(line))
		if maxScanBytes > 0 && scannedBytes > maxScanBytes {
			_ = file.Close()
			return tool.Result{}, tool.NewToolError(
				tool.ErrorCodeExecution,
				fmt.Sprintf("read line selection scanned more than %d bytes; use byte offset pagination for very large files", maxScanBytes),
			)
		}
		lineNumber++
		if lineNumber < start {
			continue
		}
		if input.EndLine > 0 && lineNumber > input.EndLine {
			break
		}

		if !utf8.Valid(line) {
			_ = file.Close()
			return tool.Result{}, tool.NewToolError(
				tool.ErrorCodeInvalidArguments,
				fmt.Sprintf("%q is not valid UTF-8 near line %d", input.Path, lineNumber),
			)
		}
		if bytes.IndexByte(line, byte(0)) >= 0 {
			_ = file.Close()
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q contains NUL bytes near line %d and is not a text artifact", input.Path, lineNumber))
		}
		prefix := ""
		if input.LineNumbers {
			prefix = fmt.Sprintf("%6d\t", lineNumber)
		}
		if output.Len()+len(prefix)+len(line) > input.Limit {
			truncated = true
			next := lineNumber
			nextLine = &next
			break
		}
		output.WriteString(prefix)
		_, _ = output.Write(line)
	}
	if err := scanner.Err(); err != nil {
		_ = file.Close()
		return tool.Result{}, fmt.Errorf("read %q by line: %w", input.Path, err)
	}
	if err := file.Close(); err != nil {
		return tool.Result{}, fmt.Errorf("close %q: %w", input.Path, err)
	}

	text := output.String()
	if truncated {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "[output truncated; narrow start_line/end_line or increase limit]"
	}
	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     text,
		Truncated:  truncated,
		Pagination: support.PaginationState(truncated, "line", nil, nextLine, ""),
	}, nil
}

func scanLinesKeepEnd(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func utf8PageCut(data []byte, target int) (int, error) {
	if target <= 0 || len(data) == 0 {
		return 0, nil
	}
	position := 0
	for position < len(data) {
		_, size := utf8.DecodeRune(data[position:])
		if size == 1 && data[position] >= utf8.RuneSelf {
			return 0, fmt.Errorf("invalid UTF-8 at page byte %d", position)
		}
		next := position + size
		if next > target {
			if position == 0 {
				return next, nil
			}
			return position, nil
		}
		position = next
		if position == target {
			return position, nil
		}
	}
	return position, nil
}
