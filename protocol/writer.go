package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// Writer provides efficient writing of RESP protocol messages
type Writer struct {
	bw *bufio.Writer
}

// NewWriter creates a new RESP protocol writer
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		bw: bufio.NewWriter(w),
	}
}

// WriteValue writes a RESP value to the output stream
func (w *Writer) WriteValue(v Value) error {
	switch v.Type {
	case TypeSimpleString:
		return w.WriteSimpleString(string(v.Data))
	case TypeError:
		return w.WriteError(string(v.Data))
	case TypeInteger:
		return w.WriteInteger(v.Integer)
	case TypeBulkString:
		if v.IsNull {
			return w.WriteNullBulkString()
		}
		return w.WriteBulkString(v.Data)
	case TypeArray:
		if v.IsNull {
			return w.WriteNullArray()
		}
		return w.WriteArray(v.Array)
	default:
		return fmt.Errorf("unsupported value type: %c", v.Type)
	}
}

// WriteSimpleString writes a simple string
func (w *Writer) WriteSimpleString(s string) error {
	if _, err := w.bw.WriteString("+"); err != nil {
		return err
	}
	if _, err := w.bw.WriteString(s); err != nil {
		return err
	}
	return w.writeCRLF()
}

// WriteError writes an error message
func (w *Writer) WriteError(msg string) error {
	if _, err := w.bw.WriteString("-"); err != nil {
		return err
	}
	if _, err := w.bw.WriteString(msg); err != nil {
		return err
	}
	return w.writeCRLF()
}

// WriteInteger writes an integer
func (w *Writer) WriteInteger(n int64) error {
	if _, err := w.bw.WriteString(":"); err != nil {
		return err
	}
	if _, err := w.bw.WriteString(strconv.FormatInt(n, 10)); err != nil {
		return err
	}
	return w.writeCRLF()
}

// WriteBulkString writes a bulk string
func (w *Writer) WriteBulkString(data []byte) error {
	if _, err := w.bw.WriteString("$"); err != nil {
		return err
	}
	if _, err := w.bw.WriteString(strconv.Itoa(len(data))); err != nil {
		return err
	}
	if err := w.writeCRLF(); err != nil {
		return err
	}
	if _, err := w.bw.Write(data); err != nil {
		return err
	}
	return w.writeCRLF()
}

// WriteBulkStringFromString writes a bulk string from a string
func (w *Writer) WriteBulkStringFromString(s string) error {
	return w.WriteBulkString([]byte(s))
}

// WriteNullBulkString writes a null bulk string
func (w *Writer) WriteNullBulkString() error {
	if _, err := w.bw.WriteString("$-1"); err != nil {
		return err
	}
	return w.writeCRLF()
}

// WriteArray writes an array of values
func (w *Writer) WriteArray(values []Value) error {
	if _, err := w.bw.WriteString("*"); err != nil {
		return err
	}
	if _, err := w.bw.WriteString(strconv.Itoa(len(values))); err != nil {
		return err
	}
	if err := w.writeCRLF(); err != nil {
		return err
	}

	for _, value := range values {
		if err := w.WriteValue(value); err != nil {
			return err
		}
	}

	return nil
}

// WriteNullArray writes a null array
func (w *Writer) WriteNullArray() error {
	if _, err := w.bw.WriteString("*-1"); err != nil {
		return err
	}
	return w.writeCRLF()
}

// WriteCommand writes a Redis command as a RESP array
func (w *Writer) WriteCommand(cmd string, args ...string) error {
	// Write array header
	if _, err := w.bw.WriteString("*"); err != nil {
		return err
	}
	if _, err := w.bw.WriteString(strconv.Itoa(1 + len(args))); err != nil {
		return err
	}
	if err := w.writeCRLF(); err != nil {
		return err
	}

	// Write command name
	if err := w.WriteBulkStringFromString(cmd); err != nil {
		return err
	}

	// Write arguments
	for _, arg := range args {
		if err := w.WriteBulkStringFromString(arg); err != nil {
			return err
		}
	}

	return nil
}

// WriteOK writes a simple "OK" response
func (w *Writer) WriteOK() error {
	return w.WriteSimpleString("OK")
}

// WritePONG writes a simple "PONG" response
func (w *Writer) WritePONG() error {
	return w.WriteSimpleString("PONG")
}

// Flush flushes any buffered data to the underlying writer
func (w *Writer) Flush() error {
	return w.bw.Flush()
}

// writeCRLF writes the CRLF terminator
func (w *Writer) writeCRLF() error {
	_, err := w.bw.WriteString(CRLF)
	return err
}

// Reset resets the writer to write to a new underlying writer
func (w *Writer) Reset(writer io.Writer) {
	w.bw.Reset(writer)
}
