package main

import (
	"io"
	"time"
)

const (
	escapeByte    = byte(27)
	interruptByte = byte(3)
)

func quitAwareInput(input io.Reader) io.ReadCloser {
	return newQuitAwareInput(input, 40*time.Millisecond)
}

func newQuitAwareInput(input io.Reader, escapeDelay time.Duration) io.ReadCloser {
	reader, writer := io.Pipe()
	bytesRead := make(chan byte, 64)

	go func() {
		defer close(bytesRead)
		buffer := make([]byte, 64)
		for {
			count, err := input.Read(buffer)
			for _, value := range buffer[:count] {
				bytesRead <- value
			}
			if err != nil {
				return
			}
		}
	}()

	go func() {
		defer writer.Close()
		write := func(values ...byte) bool {
			_, err := writer.Write(values)
			return err == nil
		}

		for value := range bytesRead {
			switch value {
			case 'q':
				write(interruptByte)
				return
			case escapeByte:
				timer := time.NewTimer(escapeDelay)
				select {
				case next, ok := <-bytesRead:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					if !ok || (next != '[' && next != 'O') {
						write(interruptByte)
						return
					}
					if !write(escapeByte, next) || !copyEscapeSequence(bytesRead, write) {
						return
					}
				case <-timer.C:
					write(interruptByte)
					return
				}
			default:
				if !write(value) {
					return
				}
			}
		}
	}()

	return reader
}

func copyEscapeSequence(input <-chan byte, write func(...byte) bool) bool {
	for value := range input {
		if !write(value) {
			return false
		}
		if value >= '@' && value <= '~' {
			return true
		}
	}
	return false
}
