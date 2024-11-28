package zip

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"text/tabwriter"
	"time"
)

type ZipDir struct {
	name             string
	stream           io.ReadWriteSeeker
	numEntries       uint16 // number of entries in the central directory
	commentLength    uint16
	comment          []byte
	fileHeaders      []FileHeader
	centralDirOffset uint32
	centralDirSize   uint32
}

type FileHeader struct {
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

func (fh *FileHeader) getDateTime() time.Time {
	return dosToTime(fh.dosDate, fh.dosTime)
}

func NewZipDir(name string, stream io.ReadWriteSeeker) (*ZipDir, error) {
	zd := ZipDir{name: name, stream: stream}
	err := zd.readDirectory()
	if err != nil {
		return nil, err
	}
	return &zd, nil
}

func (zd *ZipDir) readDirectory() error {
	// Start at the end of the file and look for the end of central directory signature
	// End of central directory is record is 22 bytes plus the zip file comment.
	// Start at EOF-22 and go backwards, looking for the end of central directory signature.
	found := false
	buffer := make([]byte, 22)
	offset := int64(-22) // offset is measured from the end of the stream
	for ; !found; offset-- {
		pos, err := zd.stream.Seek(offset, io.SeekEnd)
		if err != nil || pos < 0 {
			return errors.New("couldn't find end of directory signature (seek failed)")
		}
		err = binary.Read(zd.stream, binary.LittleEndian, &buffer)
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
	zd.numEntries = binary.LittleEndian.Uint16(buffer[10:12])
	zd.centralDirSize = binary.LittleEndian.Uint32(buffer[12:16])
	zd.centralDirOffset = binary.LittleEndian.Uint32(buffer[16:20])
	zd.commentLength = binary.LittleEndian.Uint16(buffer[20:22])
	if zd.commentLength > 0 {
		zd.comment = make([]byte, zd.commentLength)
		err := binary.Read(zd.stream, binary.LittleEndian, &zd.comment)
		if err != nil {
			return err
		}
	}

	// Read the central directory
	buffer = make([]byte, zd.centralDirSize)
	_, err := zd.stream.Seek(int64(zd.centralDirOffset), 0)
	if err != nil {
		return err
	}
	err = binary.Read(zd.stream, binary.LittleEndian, &buffer)
	if err != nil {
		return err
	}
	// buffer now contains the central directory
	i := 0
	for entry := 0; entry < int(zd.numEntries); entry++ {
		if len(buffer) < i+46 {
			return errors.New("central directory is malformed")
		}
		if buffer[i] != 0x50 || buffer[i+1] != 0x4b || buffer[i+2] != 0x01 || buffer[i+3] != 0x02 {
			return errors.New("couldn't find central directory file header signature")
		}
		fh := FileHeader{}
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
		if len(buffer) < i+46+int(fh.nameLength)+int(fh.extraLength)+int(fh.commentLength) {
			return errors.New("central directory is malformed")
		}
		fh.fileName = string(buffer[i+46 : i+46+int(fh.nameLength)])
		if fh.extraLength > 0 {
			fh.extraField = make([]byte, int(fh.extraLength))
			fh.extraField = buffer[i+46+int(fh.nameLength) : i+46+int(fh.nameLength)+int(fh.extraLength)]
		}
		if fh.commentLength > 0 {
			fh.comment = string(buffer[i+46+int(fh.nameLength)+int(fh.extraLength) : i+46+int(fh.nameLength)+int(fh.extraLength)+int(fh.commentLength)])
		}
		zd.fileHeaders = append(zd.fileHeaders, fh)
		i += 46 + int(fh.nameLength) + int(fh.extraLength) + int(fh.commentLength)
	}

	// Don't bother reading the local file headers to make sure that the central directory
	// is valid. Do that lazily, when we actually need to look at the local file headers.

	return nil
}

func (zd *ZipDir) Display(output io.Writer) {
	fmt.Printf("Archive: %s\n", zd.name)
	if zd.commentLength > 0 {
		fmt.Printf("Comment: %s\n", zd.comment)
	}

	w := new(tabwriter.Writer)
	w.Init(output, 8, 0, 1, ' ', tabwriter.AlignRight)

	fmt.Fprintln(w, "Length\tMethod\tSize\tCmpr\tDate\tTime\tCRC-32\tName\t")
	fmt.Fprintln(w, "------\t------\t------\t------\t------\t------\t------\t------\t")

	for _, fh := range zd.fileHeaders {
		compressedPercent := int(math.Floor(float64(fh.compressedSize) / float64(fh.uncompressedSize) * 100))
		dt := fh.getDateTime()
		fmt.Fprintf(w, "%d\t%s\t%d\t%d%%\t%s\t%s\t%x\t%s\t\n",
			fh.uncompressedSize,
			compressionMethodToString(fh.compressionMethod),
			fh.compressedSize,
			compressedPercent,
			dt.Format("2006-01-02"),
			dt.Format("15:04"),
			fh.crc,
			fh.fileName)
	}
	w.Flush()
}

func compressionMethodToString(compressionMethod uint16) string {
	switch compressionMethod {
	case 0:
		return "stored"
	case 8:
		return "deflated"
	default:
		return fmt.Sprintf("%d", compressionMethod)
	}
}
