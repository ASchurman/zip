package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"text/tabwriter"
	"time"
)

type zipFile struct {
	name          string
	file          *os.File
	numEntries    uint16 // number of entries in the central directory
	commentLength uint16
	comment       []byte
	fileHeaders   []fileHeader
}

type fileHeader struct {
	versionMadeBy     uint16
	versionNeeded     uint16
	flags             uint16
	compressionMethod uint16
	dosTime           uint16
	dosDate           uint16
	crc               uint32
	compressedSize    uint32
	uncompressedSize  uint32
	nameLength        uint16
	extraLength       uint16
	commentLength     uint16
	internalAttr      uint16
	externalAttr      uint32
	offsetLocalHeader uint32
	fileName          string
	extraField        []byte
	comment           string
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

func openZipFile(name string) (*zipFile, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	zf := zipFile{name: name, file: file}
	err = zf.readDirectory()
	if err != nil {
		return nil, err
	}
	return &zf, nil
}

func (zf *zipFile) close() {
	if zf.file != nil {
		zf.file.Close()
	}
}

func (zf *zipFile) readDirectory() error {
	// Start at the end of the file and look for the end of central directory signature
	fi, err := zf.file.Stat()
	if err != nil {
		return err
	}
	fileSize := fi.Size()

	// End of central directory is record is 22 bytes plus the zip file comment.
	// Start at EOF-22 and go backwards, looking for the end of central directory signature.
	found := false
	buffer := make([]byte, 22)
	offset := fileSize - 22
	for ; offset > 0 && !found; offset-- {
		_, err = zf.file.Seek(offset, 0)
		if err != nil {
			return err
		}
		err = binary.Read(zf.file, binary.LittleEndian, &buffer)
		if err != nil {
			return err
		}
		if buffer[0] == 0x50 && buffer[1] == 0x4b && buffer[2] == 0x05 && buffer[3] == 0x06 {
			found = true
			break
		}
	}
	if !found {
		return errors.New("couldn't find end of central directory signature")
	}

	// buffer contains 22 bytes of the end of central directory record, starting from signature.
	// Now read the rest of the end of central directory record.
	// Ignore anything involving a directory spanning multiple disks...
	zf.numEntries = binary.LittleEndian.Uint16(buffer[10:12])
	centralDirectorySize := binary.LittleEndian.Uint32(buffer[12:16])
	centralDirectoryOffset := binary.LittleEndian.Uint32(buffer[16:20])
	zf.commentLength = binary.LittleEndian.Uint16(buffer[20:22])
	if zf.commentLength > 0 {
		zf.comment = make([]byte, zf.commentLength)
		_, err = zf.file.Seek(offset+22, 0)
		if err != nil {
			return err
		}
		err = binary.Read(zf.file, binary.LittleEndian, &zf.comment)
		if err != nil {
			return err
		}
	}

	// Read the central directory
	buffer = make([]byte, centralDirectorySize)
	_, err = zf.file.Seek(int64(centralDirectoryOffset), 0)
	if err != nil {
		return err
	}
	err = binary.Read(zf.file, binary.LittleEndian, &buffer)
	if err != nil {
		return err
	}
	// buffer now contains the central directory
	i := 0
	for entry := 0; entry < int(zf.numEntries); entry++ {
		if buffer[i] != 0x50 || buffer[i+1] != 0x4b || buffer[i+2] != 0x01 || buffer[i+3] != 0x02 {
			return errors.New("couldn't find central directory file header signature")
		}
		fh := fileHeader{}
		fh.versionMadeBy = binary.LittleEndian.Uint16(buffer[i+4 : i+6])
		fh.versionNeeded = binary.LittleEndian.Uint16(buffer[i+6 : i+8])
		fh.flags = binary.LittleEndian.Uint16(buffer[i+8 : i+10])
		fh.compressionMethod = binary.LittleEndian.Uint16(buffer[i+10 : i+12])
		fh.dosTime = binary.LittleEndian.Uint16(buffer[i+12 : i+14])
		fh.dosDate = binary.LittleEndian.Uint16(buffer[i+14 : i+16])
		fh.crc = binary.LittleEndian.Uint32(buffer[i+16 : i+20])
		fh.compressedSize = binary.LittleEndian.Uint32(buffer[i+20 : i+24])
		fh.uncompressedSize = binary.LittleEndian.Uint32(buffer[i+24 : i+28])
		fh.nameLength = binary.LittleEndian.Uint16(buffer[i+28 : i+30])
		fh.extraLength = binary.LittleEndian.Uint16(buffer[i+30 : i+32])
		fh.commentLength = binary.LittleEndian.Uint16(buffer[i+32 : i+34])
		// don't bother with disk # start
		fh.internalAttr = binary.LittleEndian.Uint16(buffer[i+36 : i+38])
		fh.externalAttr = binary.LittleEndian.Uint32(buffer[i+38 : i+42])
		fh.offsetLocalHeader = binary.LittleEndian.Uint32(buffer[i+42 : i+46])
		fh.fileName = string(buffer[i+46 : i+46+int(fh.nameLength)])
		if fh.extraLength > 0 {
			fh.extraField = make([]byte, int(fh.extraLength))
			fh.extraField = buffer[i+46+int(fh.nameLength) : i+46+int(fh.nameLength)+int(fh.extraLength)]
		}
		if fh.commentLength > 0 {
			fh.comment = string(buffer[i+46+int(fh.nameLength)+int(fh.extraLength) : i+46+int(fh.nameLength)+int(fh.extraLength)+int(fh.commentLength)])
		}
		zf.fileHeaders = append(zf.fileHeaders, fh)
		i += 46 + int(fh.nameLength) + int(fh.extraLength) + int(fh.commentLength)
	}

	// Don't bother reading the local file headers to make sure that the central directory
	// is valid.

	return nil
}

func (zf *zipFile) display() {
	fmt.Printf("Archive: %s\n\n", zf.name)
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 8, 0, 1, ' ', tabwriter.AlignRight)

	fmt.Fprintln(w, "Length\tMethod\tSize\tCmpr\tDate\tTime\tCRC-32\tName\t")
	fmt.Fprintln(w, "------\t------\t------\t------\t------\t------\t------\t------\t")

	for _, fh := range zf.fileHeaders {
		compressedPercent := int(math.Floor(float64(fh.compressedSize) / float64(fh.uncompressedSize) * 100))
		dt := fh.getDateTime()
		fmt.Fprintf(w, "%d\t%d\t%d\t%d%%\t%s\t%s\t%x\t%s\t\n",
			fh.uncompressedSize,
			fh.compressionMethod,
			fh.compressedSize,
			compressedPercent,
			dt.Format("2006-01-02"),
			dt.Format("15:04"),
			fh.crc,
			fh.fileName)
	}
	w.Flush()
}
