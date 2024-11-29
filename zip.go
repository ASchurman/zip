package zip

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"text/tabwriter"

	"github.com/spf13/afero"
)

// File represents a zip file. It contains fields for I/O (fs, name, file),
// fields corresponding to the end-of-central-directory record (numEntries,
// centralDirSize, centralDirOffset, commentLength, comment), and a slice
// of file headers from the central directory (fileHeaders).
type File struct {
	fs               afero.Fs     // Use afero for the sake of testing
	Name             string       // zip file name
	file             afero.File   // file handle for the archive
	numEntries       uint16       // number of entries in the central directory
	centralDirSize   uint32       // size of the central directory
	centralDirOffset uint32       // offset of the central directory, relative to the start of the file
	commentLength    uint16       // length of the zip file comment
	comment          []byte       // zip file comment
	fileHeaders      []fileHeader // file headers from the central directory
}

// FileHeader represents a file header from the zip file's central directory. Each field
// corresponds to a field in the zip file's central directory file header.
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

func Create(archiveName string, fileName string, method CompressionMethod) (*File, error) {
	return CreateWithFs(archiveName, fileName, method, afero.NewOsFs())
}

func CreateWithFs(archiveName string, fileName string, method CompressionMethod, fs afero.Fs) (*File, error) {
	zf := File{Name: archiveName, fs: fs}
	file, err := zf.fs.Create(archiveName)
	if err != nil {
		return nil, err
	}

	zf.file = file
	zf.numEntries = 0
	zf.commentLength = 0
	zf.centralDirSize = CENTRAL_DIR_MIN_SIZE
	zf.centralDirOffset = 0

	err = zf.AddFile(fileName, method)
	return &zf, err
}

// Open opens an existing zip file with the given name and returns a zip.File
// that can be used to interact with the zip file.
func Open(name string) (*File, error) {
	return OpenWithFs(name, afero.NewOsFs())
}

// OpenWithFs opens an existing zip file with the given name and returns a zip.File
// that can be used to interact with the zip file. The given afero.Fs is used instead
// of the default os file system.
func OpenWithFs(name string, fs afero.Fs) (*File, error) {
	zf := File{Name: name, fs: fs}
	file, err := zf.fs.Open(name)
	if err != nil {
		return nil, err
	}
	zf.file = file

	err = zf.readDirectory()
	if err != nil {
		return nil, err
	}
	return &zf, nil
}

// Close closes the underlying file associated with the zip.File.
// It returns an error if the file cannot be closed.
func (zf *File) Close() error {
	return zf.file.Close()
}

// readDirectory reads the central directory of a zip file to populate the
// File struct with metadata about the archive's contents. It seeks from the
// end of the file to locate the end-of-central-directory signature, reads
// the central directory records, and extracts file headers into a slice.
// It also handles reading the archive's comment if present. Errors are
// returned if the directory signature cannot be found or if the directory
// structure is malformed.
func (zf *File) readDirectory() error {
	// Start at the end of the file and look for the end of central directory signature
	// End of central directory is record is 22 bytes plus the zip file comment.
	// Start at EOF-22 and go backwards, looking for the end of central directory signature.
	found := false
	buffer := make([]byte, 22)
	offset := int64(-22) // offset is measured from the end of the stream
	for ; !found; offset-- {
		pos, err := zf.file.Seek(offset, io.SeekEnd)
		if err != nil || pos < 0 {
			return newZipErrorStr("Read Directory", "couldn't find end of directory signature")
		}
		err = binary.Read(zf.file, binary.LittleEndian, &buffer)
		if err != nil {
			return newZipError("Read Directory", err)
		}
		if buffer[0] == 0x50 && buffer[1] == 0x4b && buffer[2] == 0x05 && buffer[3] == 0x06 {
			found = true
			break
		}
	}
	if !found {
		return newZipErrorStr("Read Directory", "couldn't find end of central directory signature")
	}

	// buffer contains 22 bytes of the end of central directory record, starting from signature.
	// Now read the rest of the end of central directory record.
	// Ignore anything involving a directory spanning multiple disks...
	zf.numEntries = binary.LittleEndian.Uint16(buffer[10:12])
	zf.centralDirSize = binary.LittleEndian.Uint32(buffer[12:16])
	zf.centralDirOffset = binary.LittleEndian.Uint32(buffer[16:20])
	zf.commentLength = binary.LittleEndian.Uint16(buffer[20:22])
	if zf.commentLength > 0 {
		zf.comment = make([]byte, zf.commentLength)
		err := binary.Read(zf.file, binary.LittleEndian, &zf.comment)
		if err != nil {
			return newZipError("Read Directory", err)
		}
	}

	// Read the central directory
	buffer = make([]byte, zf.centralDirSize)
	_, err := zf.file.Seek(int64(zf.centralDirOffset), 0)
	if err != nil {
		return newZipError("Read Directory", err)
	}
	err = binary.Read(zf.file, binary.LittleEndian, &buffer)
	if err != nil {
		return newZipError("Read Directory", err)
	}
	// buffer now contains the central directory
	i := 0
	for entry := 0; entry < int(zf.numEntries); entry++ {
		if len(buffer) < i+46 {
			return newZipErrorStr("Read Directory", "central directory is malformed")
		}
		if buffer[i] != 0x50 || buffer[i+1] != 0x4b || buffer[i+2] != 0x01 || buffer[i+3] != 0x02 {
			return newZipErrorStr("Read Directory", "couldn't find central directory file header signature")
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
		if len(buffer) < i+46+int(fh.nameLength)+int(fh.extraLength)+int(fh.commentLength) {
			return newZipErrorStr("Read Directory", "central directory is malformed")
		}
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
	// is valid. Do that lazily, when we actually need to look at the local file headers.

	// TODO maybe check the local file headers here, actually. For the caller, it makes
	// the most sense to get an error when you first try to open a bad zip file, vs
	// when you try to extract a file from it.

	return nil
}

// Display prints out a table of contents for the zip file to the given Writer.
// The table of contents format is similar to the "unzip -v" command.
func (zf *File) Display(output io.Writer) {
	fmt.Printf("Archive: %s\n", zf.Name)
	if zf.commentLength > 0 {
		fmt.Printf("Comment: %s\n", zf.comment)
	}

	w := new(tabwriter.Writer)
	w.Init(output, 8, 0, 1, ' ', tabwriter.AlignRight)

	fmt.Fprintln(w, "Length\tMethod\tSize\tCmpr\tDate\tTime\tCRC-32\tName\t")
	fmt.Fprintln(w, "------\t------\t------\t------\t------\t------\t------\t------\t")

	for _, fh := range zf.fileHeaders {
		compressedPercent := int(math.Floor(float64(fh.compressedSize) / float64(fh.uncompressedSize) * 100))
		dt := fh.getDateTime()
		fmt.Fprintf(w, "%d\t%s\t%d\t%d%%\t%s\t%s\t%x\t%s\t\n",
			fh.uncompressedSize,
			compressionMethodToString(CompressionMethod(fh.compressionMethod)),
			fh.compressedSize,
			compressedPercent,
			dt.Format("2006-01-02"),
			dt.Format("15:04"),
			fh.crc,
			fh.fileName)
	}
	w.Flush()
}

func (zf *File) AddFile(name string, method CompressionMethod) error {
	if method == COMPRESS_DEFLATED {
		return errors.New("deflate not implemented")
	}

	// Make a temp file to write the new zip contents into

	// Traverse the current file, copying it to the temp file unless it's the file we're adding.

	// Update metadata with the new file

	// Write the new file we're adding

	// Write the central directory

	// Clean-up:
	// Close the temp file, close and delete zf.file, rename the temp file,
	// replace zf.file with the renamed temp file, and reopen it.

	return errors.New("not implemented")
}

func (zf *File) RemoveFile(name string) error {
	// Make a temp file to write the new zip contents into

	// Traverse the current file, copying it to the temp file unless it's the file we're removing.

	// Update metadata with the removed file

	// Write the central directory

	// Clean-up:
	// Close the temp file, close and delete zf.file, rename the temp file,
	// replace zf.file with the renamed temp file, and reopen it.

	return errors.New("not implemented")
}

func (zf *File) ExtractFile(name string) error {
	for _, fh := range zf.fileHeaders {
		if fh.fileName == name {
			return zf.extractSingleFile(&fh)
		}
	}
	return errors.New("file not found")
}

func (zf *File) ExtractAll() error {
	for _, fh := range zf.fileHeaders {
		err := zf.extractSingleFile(&fh)
		if err != nil {
			return err
		}
	}
	return nil
}

func (zf *File) extractSingleFile(fh *fileHeader) error {
	if fh.compressionMethod == COMPRESS_DEFLATED {
		return errors.New("deflate not implemented")
	}

	// read extra field length so that we can seek to the file data
	_, err := zf.file.Seek(int64(fh.offsetLocalHeader+28), io.SeekStart)
	if err != nil {
		return err
	}
	var extraFieldLength uint16
	err = binary.Read(zf.file, binary.LittleEndian, &extraFieldLength)
	if err != nil {
		return err
	}

	// seek past file name and extra field to get to file data
	offsetFromCurrent := fh.nameLength + extraFieldLength
	_, err = zf.file.Seek(int64(offsetFromCurrent), io.SeekCurrent)
	if err != nil {
		return err
	}

	// file pointer is now at the start of the file data. Read fh.compressedSize bytes from
	// zf.file and write them to outfile.
	outfileTempName := tempName(fh.fileName)
	outfile, err := zf.fs.Create(outfileTempName)
	if err != nil {
		return err
	}
	_, err = io.Copy(outfile, io.LimitReader(zf.file, int64(fh.compressedSize)))
	if err != nil {
		zf.closeAndDeleteTempFile(outfile, outfileTempName)
		return err
	}
	return zf.closeAndRenameTempFile(outfile, outfileTempName, fh.fileName)
}
