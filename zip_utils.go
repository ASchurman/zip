package zip

import (
	"encoding/binary"
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

	// Constants for file headers that we make from scratch
	VERSION_MADE_BY = 20
	VERSION_NEEDED  = 20
	FLAGS           = 0
	INTERNAL_ATTR   = 0
	EXTERNAL_ATTR   = 0
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

func timeToDosDateTime(t time.Time) (uint16, uint16) {
	year := uint16(t.Year() - 1980)
	month := uint16(t.Month())
	day := uint16(t.Day())
	hr := uint16(t.Hour())
	min := uint16(t.Minute())
	sec := uint16(t.Second())

	dosDate := uint16(year<<9 | month<<5 | day)
	dosTime := uint16(hr<<11 | min<<5 | sec)

	return dosDate, dosTime
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
	fileCrc, err := getCrc(file)
	if err != nil {
		return false, err
	}
	return crc == fileCrc, nil
}

func getCrc(file afero.File) (uint32, error) {
	// TODO there's surely a better way to do this beside reading the whole file into memory
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	data := make([]byte, info.Size())
	_, err = file.Read(data)
	if err != nil {
		return 0, err
	}
	return crc32.ChecksumIEEE(data), nil
}

// Writes the zip archive to the temporary new zip file.
// Assumes that zf.fileHeaders has the correct headers in it, but fields related to
// offsets and the size of the central directory are incorrect.
// newFh and newData represent a new file to be added to the archive. If they're not nil,
// then they will be added.
func (zf *File) writeArchive(outfile afero.File, newFh *fileHeader, newData io.ReadSeeker) error {
	_, err := outfile.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	if newFh != nil {
		zf.fileHeaders = append(zf.fileHeaders, *newFh)
	}

	// Write local file headers and file data
	for i, fh := range zf.fileHeaders {
		// Get the data for this header's file BEFORE we change anything about the header
		var fileData io.Reader
		if newFh == nil || i < len(zf.fileHeaders)-1 {
			fileDataOffset := fh.offsetLocalHeader + 30 + uint32(fh.nameLength) + uint32(fh.extraLengthLocal)
			zf.file.Seek(int64(fileDataOffset), io.SeekStart)
			fileData = io.LimitReader(zf.file, int64(fh.compressedSize))
		} else { // Last file, which is the new file, since newFh != nil
			newData.Seek(0, io.SeekStart)
			fileData = newData
		}

		// Update the file header struct: offset and extra length
		offset, err := outfile.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		zf.fileHeaders[i].offsetLocalHeader = uint32(offset)
		zf.fileHeaders[i].extraLengthLocal = 0

		binary.Write(outfile, binary.LittleEndian, []byte("\x50\x4b\x03\x04"))
		binary.Write(outfile, binary.LittleEndian, fh.versionNeeded)
		binary.Write(outfile, binary.LittleEndian, fh.flags)
		binary.Write(outfile, binary.LittleEndian, fh.compressionMethod)
		binary.Write(outfile, binary.LittleEndian, fh.dosTime)
		binary.Write(outfile, binary.LittleEndian, fh.dosDate)
		binary.Write(outfile, binary.LittleEndian, fh.crc)
		binary.Write(outfile, binary.LittleEndian, fh.compressedSize)
		binary.Write(outfile, binary.LittleEndian, fh.uncompressedSize)
		binary.Write(outfile, binary.LittleEndian, fh.nameLength)
		binary.Write(outfile, binary.LittleEndian, []byte("\x00\x00")) // extra field length
		binary.Write(outfile, binary.LittleEndian, []byte(fh.fileName))
		// Extra field goes after file name, but we're not keeping extra fields
		_, err = io.Copy(outfile, fileData)
		if err != nil {
			return err
		}
	}

	// Update central directory offset and write central directory
	offset, err := outfile.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	zf.centralDirOffset = uint32(offset)

	for i, fh := range zf.fileHeaders {
		errs := []error{}
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, []byte("\x50\x4b\x01\x02")))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.versionMadeBy))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.versionNeeded))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.flags))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.compressionMethod))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.dosTime))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.dosDate))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.crc))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.compressedSize))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.uncompressedSize))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.nameLength))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, []byte("\x00\x00"))) // extra field length
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.commentLength))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, uint16(0))) // disk # start
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.internalAttr))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.externalAttr))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, fh.offsetLocalHeader))
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, []byte(fh.fileName)))
		// Extra field goes after file name, but we're not keeping extra fields
		errs = append(errs, binary.Write(outfile, binary.LittleEndian, []byte(fh.comment)))
		for _, err := range errs {
			if err != nil {
				return err
			}
		}
		zf.fileHeaders[i].extraLengthCentral = 0 // We threw out the extra field as we wrote the central directory
	}

	// Update central directory size
	offset, err = outfile.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	zf.centralDirSize = uint32(offset) - zf.centralDirOffset

	// Write the end-of-central-directory record
	errs := []error{}
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, []byte("\x50\x4b\x05\x06")))
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, uint16(0)))     // disk # start
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, uint16(0)))     // disk # of cd
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, zf.numEntries)) // entires on this disk
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, zf.numEntries)) // total entries
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, zf.centralDirSize))
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, zf.centralDirOffset))
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, zf.commentLength))
	errs = append(errs, binary.Write(outfile, binary.LittleEndian, zf.comment))
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}
