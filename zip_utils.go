package zip

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"time"

	"github.com/spf13/afero"
)

// CompressionMethod is a uint16 corresponding to the compression method field
// in a zip file.
type CompressionMethod uint16

const (
	COMPRESS_STORED   = 0
	COMPRESS_DEFLATED = 8
)

func compressionMethodToString(method CompressionMethod) string {
	switch method {
	case COMPRESS_STORED:
		return "stored"
	case COMPRESS_DEFLATED:
		return "deflated"
	default:
		return fmt.Sprintf("%d", method)
	}
}

const (
	// If we have no files, then we only have end-of-central-dir record
	CENTRAL_DIR_MIN_SIZE = 22
)

type ZipError struct {
	Operation string
	Err       error
}

func (e *ZipError) Error() string {
	return fmt.Sprintf("%s: %s", e.Operation, e.Err.Error())
}

func newZipError(operation string, err error) *ZipError {
	return &ZipError{Operation: operation, Err: err}
}
func newZipErrorStr(operation string, errStr string) *ZipError {
	return &ZipError{Operation: operation, Err: errors.New(errStr)}
}

func dosToTime(dosDate uint16, dosTime uint16) time.Time {
	sec := dosTime & 0x1f
	min := (dosTime >> 5) & 0x3f
	hr := (dosTime >> 11) & 0x1f
	day := dosDate & 0x1f
	month := (dosDate >> 5) & 0xf
	year := (dosDate >> 9) & 0x7f

	return time.Date(int(year)+1980, time.Month(month), int(day), int(hr), int(min), int(sec), 0, time.Local)
}

func (fh *fileHeader) getDateTime() time.Time {
	return dosToTime(fh.dosDate, fh.dosTime)
}

// Make a a temp file name for the given fileName. Keep this code in one place
// for the sake of keeping it standard.
func tempName(fileName string) string {
	return fmt.Sprintf("%s.tmp", fileName)
}

func (zf *File) closeAndDeleteTempFile(file afero.File, name string) error {
	err := file.Close()
	if err != nil {
		return err
	}
	return zf.fs.Remove(name)
}

func (zf *File) closeAndRenameTempFile(file afero.File, tempName string, name string) error {
	err := file.Close()
	if err != nil {
		return err
	}
	return zf.fs.Rename(tempName, name) // Will replace any file with the same name!
}

func checkCrc(crc uint32, file afero.File) (bool, error) {
	// TODO there's surely a better way to do this beside reading the whole file into memory
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return false, err
	}
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	data := make([]byte, info.Size())
	_, err = file.Read(data)
	if err != nil {
		return false, err
	}
	fileCrc := crc32.ChecksumIEEE(data)

	return crc == fileCrc, nil
}
