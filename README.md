Let's implement Zip in Go for fun! Because using Go is a delight.

For now, only file storage without compression is supported. Implementing deflate compression is the next step.

## Usage
Run from the command line:
```
zip {-d|-r|-t|-x} ARCHIVE [FILE ...]
```

* `ARCHIVE`: The zip archive on which to operate.
* `-d`: Deletes the provided FILE(s) from the archive.
* `-r`: Adds the provided FILE(s) to the archive, or replaces them if files with the same name already exist in the archive. If the archive doesn't yet exist, creates a new zip file.
* `-t`: Prints a table listing the files in the archive.
* `-x`: Extracts the provided FILE from the archive.
