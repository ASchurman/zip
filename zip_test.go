package zip

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func makeTestFile(fs afero.Fs, name string, data []byte) error {
	file, err := fs.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	err = file.Close()
	if err != nil {
		return err
	}
	return nil
}

type testfile struct {
	name    string
	comment string
	data    []byte
}

func makeZipFile(t *testing.T, fs afero.Fs, zipname string, comment string, files []testfile) {
	// Create test zip file using a different zip writer (so this test doesn't depend
	// on my implementation of adding files to zip archive)
	zipFile, err := fs.Create(zipname)
	if err != nil {
		t.Fatalf("fs.Create returned error: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, f := range files {
		header := zip.FileHeader{
			Name:           f.name,
			Comment:        f.comment,
			Modified:       time.Now(),
			Method:         zip.Store,
			CreatorVersion: 0x20,
			ReaderVersion:  0x20,
		}
		writer, err := zipWriter.CreateHeader(&header)
		if err != nil {
			t.Fatalf("zipWriter.CreateHeader returned error: %v", err)
		}
		_, err = writer.Write(f.data)
		if err != nil {
			t.Fatalf("writer.Write returned error: %v", err)
		}
	}
	zipWriter.SetComment(comment)
}

// Confirms that a zip file contains the expected files
func verifyZipFile(t *testing.T, fs afero.Fs, zipname string, expZipComment string, expFiles []testfile) error {
	zipFile, err := fs.Open(zipname)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	info, err := zipFile.Stat()
	if err != nil {
		return err
	}
	zipReader, err := zip.NewReader(zipFile, info.Size())
	if err != nil {
		return err
	}

	if zipReader.Comment != expZipComment {
		t.Errorf("zipReader.Comment is %q; Want: %q", zipReader.Comment, expZipComment)
	}
	if len(zipReader.File) != len(expFiles) {
		t.Errorf("zipReader.File has length %d; Want: %d", len(zipReader.File), len(expFiles))
	}

	// Verify each file in the zip
	for i, f := range zipReader.File {
		if f.Name != expFiles[i].name {
			t.Errorf("zipReader.File[%d].Name is %q; Want: %q", i, f.Name, expFiles[i].name)
		}
		if f.Comment != expFiles[i].comment {
			t.Errorf("zipReader.File[%d].Comment is %q; Want: %q", i, f.Comment, expFiles[i].comment)
		}
		fFile, err := f.Open()
		if err != nil {
			return err
		}
		fData := make([]byte, len(expFiles[i].data))
		fDataSize, err := fFile.Read(fData)
		if err != nil {
			fFile.Close()
			return err
		}
		if fDataSize != len(expFiles[i].data) {
			t.Errorf("zipReader.File[%d].Data has length %d; Want: %d", i, fDataSize, len(expFiles[i].data))
		}
		if !reflect.DeepEqual(fData, expFiles[i].data) {
			t.Errorf("zipReader.File[%d].Data is %x; Want: %x", i, fData, expFiles[i].data)
		}
		fFile.Close()
	}
	return nil
}

// Confirms that a file contains the expected data
func verifyFile(t *testing.T, fs afero.Fs, name string, data []byte) error {
	file, err := fs.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	fileData := make([]byte, len(data))
	fileDataSize, err := file.Read(fileData)
	if err != nil {
		return err
	}
	if fileDataSize != len(data) {
		t.Errorf("fileData has length %d; Want: %d", fileDataSize, len(data))
	}
	if !reflect.DeepEqual(fileData, data) {
		t.Errorf("fileData is %v; Want: %v", fileData, data)
	}
	return nil
}

func TestReadDirectoryFailures(t *testing.T) {
	appFs := afero.NewMemMapFs()
	var testcases = []struct {
		name      string
		expectErr bool
		data      []byte
	}{
		{"WellFormed", false, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00\x00")},
		{"WellFormedWithComment", false, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x07\x00\x43\x6f\x6d\x6d\x65\x6e\x74")},
		{"CommentLengthTooBig", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x08\x00\x43\x6f\x6d\x6d\x65\x6e\x74")},
		{"NoEndOfDirSignature", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x00\x00\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00\x00")},
		{"TooShortForEndOfDirRecord", true, []byte("\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00")},
		{"CentralDirOffsetTooBig", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x7a\x00\x00\x00\x00\x00")},
		{"NoCentralDirSignature", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x00\x00\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00\x00")},
		{"FileNameTooLong", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\xff\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00\x00")},
		{"ExtraFieldTooLong", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\xff\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00\x00")},
		{"FileCommentTooLong", true, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x54\x65\x73\x74\x42\x6f\x64\x79\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x4b\x7c\x59\xe7\x03\xfa\xb6\x08\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00\xff\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x74\x65\x73\x74\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x36\x00\x00\x00\x2e\x00\x00\x00\x00\x00")},
	}

	for _, c := range testcases {
		t.Run(c.name, func(t *testing.T) {
			// Create test file
			err := makeTestFile(appFs, c.name, c.data)
			if err != nil {
				t.Fatalf("makeTestFile returned error: %v", err)
			}

			// Make ZipDir
			zf, err := OpenWithFs(c.name, appFs)
			if err == nil && c.expectErr {
				t.Errorf("NewZipDir should have returned error but didn't")
			} else if err != nil && !c.expectErr {
				t.Errorf("NewZipDir should not have returned error but did: %v", err)
			}
			if err == nil {
				zf.Close()
			}
		})
	}
}

func TestReadDirectory(t *testing.T) {
	appFs := afero.NewMemMapFs()

	name := "TestReadDirectory.zip"
	data := []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x84\x4a\x7e\x59\x1c\x95\x68\xa6\x05\x00\x00\x00\x05\x00\x00\x00\x09\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x62\x6f\x64\x79\x31\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x88\x4a\x7e\x59\xa6\xc4\x61\x3f\x05\x00\x00\x00\x05\x00\x00\x00\x09\x00\x00\x00\x66\x69\x6c\x65\x32\x2e\x74\x78\x74\x62\x6f\x64\x79\x32\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x8c\x4a\x7e\x59\x30\xf4\x66\x48\x05\x00\x00\x00\x05\x00\x00\x00\x09\x00\x00\x00\x66\x69\x6c\x65\x33\x2e\x74\x78\x74\x62\x6f\x64\x79\x33\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x84\x4a\x7e\x59\x1c\x95\x68\xa6\x05\x00\x00\x00\x05\x00\x00\x00\x09\x00\x00\x00\x0e\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x43\x6f\x6d\x6d\x65\x6e\x74\x4f\x6e\x46\x69\x6c\x65\x31\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x88\x4a\x7e\x59\xa6\xc4\x61\x3f\x05\x00\x00\x00\x05\x00\x00\x00\x09\x00\x00\x00\x0e\x00\x00\x00\x01\x00\x20\x00\x00\x00\x2c\x00\x00\x00\x66\x69\x6c\x65\x32\x2e\x74\x78\x74\x43\x6f\x6d\x6d\x65\x6e\x74\x4f\x6e\x46\x69\x6c\x65\x32\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x8c\x4a\x7e\x59\x30\xf4\x66\x48\x05\x00\x00\x00\x05\x00\x00\x00\x09\x00\x00\x00\x0e\x00\x00\x00\x01\x00\x20\x00\x00\x00\x58\x00\x00\x00\x66\x69\x6c\x65\x33\x2e\x74\x78\x74\x43\x6f\x6d\x6d\x65\x6e\x74\x4f\x6e\x46\x69\x6c\x65\x33\x50\x4b\x05\x06\x00\x00\x00\x00\x03\x00\x03\x00\xcf\x00\x00\x00\x84\x00\x00\x00\x0e\x00\x41\x72\x63\x68\x69\x76\x65\x43\x6f\x6d\x6d\x65\x6e\x74")
	numEntries := uint16(3)
	commentLength := uint16(14)
	comment := []byte("ArchiveComment")
	centralDirOffset := uint32(132)
	centralDirSize := uint32(207)
	headers := []fileHeader{
		{
			versionMadeBy:      0x0014,
			versionNeeded:      0x0014,
			flags:              0x0000,
			compressionMethod:  0x0000,
			dosTime:            0x4a84,
			dosDate:            0x597e,
			crc:                0xa668951c,
			compressedSize:     0x00000005,
			uncompressedSize:   0x00000005,
			nameLength:         0x0009,
			extraLengthLocal:   0x0000,
			extraLengthCentral: 0x0000,
			commentLength:      0x000e,
			internalAttr:       0x0001,
			externalAttr:       0x00000020,
			offsetLocalHeader:  0x00000000,
			fileName:           "file1.txt",
			comment:            "CommentOnFile1",
		},
		{
			versionMadeBy:      0x0014,
			versionNeeded:      0x0014,
			flags:              0x0000,
			compressionMethod:  0x0000,
			dosTime:            0x4a88,
			dosDate:            0x597e,
			crc:                0x3f61c4a6,
			compressedSize:     0x00000005,
			uncompressedSize:   0x00000005,
			nameLength:         0x0009,
			extraLengthLocal:   0x0000,
			extraLengthCentral: 0x0000,
			commentLength:      0x000e,
			internalAttr:       0x0001,
			externalAttr:       0x00000020,
			offsetLocalHeader:  0x0000002c,
			fileName:           "file2.txt",
			comment:            "CommentOnFile2",
		},
		{
			versionMadeBy:      0x0014,
			versionNeeded:      0x0014,
			flags:              0x0000,
			compressionMethod:  0x0000,
			dosTime:            0x4a8c,
			dosDate:            0x597e,
			crc:                0x4866f430,
			compressedSize:     0x00000005,
			uncompressedSize:   0x00000005,
			nameLength:         0x0009,
			extraLengthLocal:   0x0000,
			extraLengthCentral: 0x0000,
			commentLength:      0x000e,
			internalAttr:       0x0001,
			externalAttr:       0x00000020,
			offsetLocalHeader:  0x00000058,
			fileName:           "file3.txt",
			comment:            "CommentOnFile3",
		},
	}

	// Create test file
	file, err := appFs.Create(name)
	if err != nil {
		t.Fatalf("afero.Create returned error: %v", err)
	}
	_, err = file.Write(data)
	if err != nil {
		t.Fatalf("afero.Write returned error: %v", err)
	}
	file.Close()

	// Open zip File
	zf, err := OpenWithFs(name, appFs)
	if err != nil {
		t.Fatalf("OpenWithFs returned error: %v", err)
	}

	// Compare end of central directory record values
	if zf.numEntries != numEntries {
		t.Errorf("zf.numEntries = %d; want %d", zf.numEntries, numEntries)
	}
	if zf.commentLength != commentLength {
		t.Errorf("zf.commentLength = %d; want %d", zf.commentLength, commentLength)
	}
	if !bytes.Equal(zf.comment, comment) {
		t.Errorf("zf.comment = %q; want %q", zf.comment, comment)
	}
	if zf.centralDirOffset != centralDirOffset {
		t.Errorf("zf.centralDirOffset = %d; want %d", zf.centralDirOffset, centralDirOffset)
	}
	if zf.centralDirSize != centralDirSize {
		t.Errorf("zf.centralDirSize = %d; want %d", zf.centralDirSize, centralDirSize)
	}

	// Compare file headers in the central directory
	if len(zf.fileHeaders) != len(headers) {
		t.Errorf("len(zf.fileHeaders) = %d; want %d", len(zf.fileHeaders), len(headers))
	}
	if !reflect.DeepEqual(zf.fileHeaders, headers) {
		t.Errorf("zf.fileHeaders:\n %v\nWant:\n%v", zf.fileHeaders, headers)
	}
}

func TestExtract(t *testing.T) {
	appFs := afero.NewMemMapFs()

	file1Name := "file1.txt"
	file1Data := []byte("test data 1")

	var testcases = []struct {
		testName  string
		expectErr bool
		fileName  string
		fileData  []byte
		zipData   []byte
	}{
		{"WellFormed", false, file1Name, file1Data, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x72\x7d\x59\xba\xa7\x1e\x62\x0b\x00\x00\x00\x0b\x00\x00\x00\x09\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x74\x65\x73\x74\x20\x64\x61\x74\x61\x20\x31\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x72\x7d\x59\xba\xa7\x1e\x62\x0b\x00\x00\x00\x0b\x00\x00\x00\x09\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x37\x00\x00\x00\x32\x00\x00\x00\x00\x00")},
		{"BadExtraField", true, file1Name, file1Data, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x72\x7d\x59\xba\xa7\x1e\x62\x0b\x00\x00\x00\x0b\x00\x00\x00\x09\x00\x01\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x74\x65\x73\x74\x20\x64\x61\x74\x61\x20\x31\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x72\x7d\x59\xba\xa7\x1e\x62\x0b\x00\x00\x00\x0b\x00\x00\x00\x09\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x37\x00\x00\x00\x32\x00\x00\x00\x00\x00")},

		// CRC is the saame in local header and central directory, but it's incorrect:
		{"BadCRC", true, file1Name, file1Data, []byte("\x50\x4b\x03\x04\x14\x00\x00\x00\x00\x00\x22\x72\x7d\x59\xba\xa7\xff\x62\x0b\x00\x00\x00\x0b\x00\x00\x00\x09\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x74\x65\x73\x74\x20\x64\x61\x74\x61\x20\x31\x50\x4b\x01\x02\x14\x00\x14\x00\x00\x00\x00\x00\x22\x72\x7d\x59\xba\xa7\xff\x62\x0b\x00\x00\x00\x0b\x00\x00\x00\x09\x00\x00\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x00\x00\x00\x00\x66\x69\x6c\x65\x31\x2e\x74\x78\x74\x50\x4b\x05\x06\x00\x00\x00\x00\x01\x00\x01\x00\x37\x00\x00\x00\x32\x00\x00\x00\x00\x00")},
	}

	for _, c := range testcases {
		t.Run(c.testName, func(t *testing.T) {
			// Create test zip file
			err := makeTestFile(appFs, c.testName, c.zipData)
			if err != nil {
				t.Errorf("makeTestFile returned error: %v", err)
			}

			// Open the test zip file
			zf, err := OpenWithFs(c.testName, appFs)
			if err != nil {
				t.Errorf("OpenWithFs returned error: %v", err)
			}
			if zf == nil {
				t.Errorf("OpenWithFs returned nil without having an error")
			}

			// Extract file
			err = zf.ExtractFile(c.fileName)
			if c.expectErr && err == nil {
				t.Errorf("ExtractFile should have failed, but didn't")
			} else if !c.expectErr && err != nil {
				t.Errorf("ExtractFile should have succeeded, but failed: %v", err)
			}

			// Compare file data
			data, err := afero.ReadFile(appFs, c.fileName)
			if err != nil {
				t.Errorf("afero.ReadFile returned error: %v", err)
			}
			if !bytes.Equal(data, c.fileData) {
				t.Errorf("afero.ReadFile returned: \"%q\"; Want: \"%q\"", data, c.fileData)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	fs := afero.NewMemMapFs()
	zipFileName := "testArchive.zip"
	files := []testfile{
		{"file1.txt", "", []byte("This archive contains some text files.")},
		{"filebeta.txt", "", []byte("Second file in the archive.")},
		{"fileThree.txt", "", []byte("File number 3")},
	}

	makeZipFile(t, fs, zipFileName, "", files)

	zf, err := OpenWithFs(zipFileName, fs)
	if err != nil {
		t.Fatalf("OpenWithFs returned error: %v", err)
	}
	err = zf.RemoveFile(files[0].name)
	if err != nil {
		t.Fatalf("RemoveFile returned error: %v", err)
	}
	err = zf.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	verifyZipFile(t, fs, zipFileName, "", files[1:])
}

func TestAdd(t *testing.T) {
	fs := afero.NewMemMapFs()
	zipFileName := "testArchive.zip"
	files := []testfile{
		{"file1.txt", "", []byte("This archive contains some text files.")},
		{"filebeta.txt", "", []byte("Second file in the archive.")},
		{"fileThree.txt", "", []byte("File number 3")},
	}

	fileToAdd := testfile{"fileFour.txt", "", []byte("File number 4")}
	makeTestFile(fs, fileToAdd.name, fileToAdd.data)

	makeZipFile(t, fs, zipFileName, "", files)

	zf, err := OpenWithFs(zipFileName, fs)
	if err != nil {
		t.Fatalf("OpenWithFs returned error: %v", err)
	}

	err = zf.AddFile(fileToAdd.name, COMPRESS_STORED)
	if err != nil {
		t.Fatalf("AddFile returned error: %v", err)
	}
	err = zf.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	files = append(files, fileToAdd)
	verifyZipFile(t, fs, zipFileName, "", files)
}
