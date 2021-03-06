package index

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/MG-RAST/Shock/shock-server/conf"
	"github.com/MG-RAST/Shock/shock-server/node/file/format/line"
	"io"
	"math/rand"
	"os"
)

type column struct {
	f     *os.File
	r     line.LineReader
	Index *Idx
}

func NewColumnIndexer(f *os.File) column {
	return column{
		f:     f,
		r:     line.NewReader(f),
		Index: New(),
	}
}

func (c *column) Create(string) (count int64, format string, err error) {
	return
}

func CreateColumnIndex(c *column, column int, ofile string) (count int64, format string, err error) {
	tmpFilePath := fmt.Sprintf("%s/temp/%d%d.idx", conf.PATH_DATA, rand.Int(), rand.Int())

	f, err := os.Create(tmpFilePath)
	if err != nil {
		return
	}
	defer f.Close()

	format = "array"
	eof := false     // identifies EOF
	curr := int64(0) // stores the offset position of the current index
	count = 0        // stores the number of indexed positions and get returned
	total_n := 0     // stores the number of bytes read for the current index record
	line_count := 0  // stores the number of lines that have been read from the data file
	prev_str := ""   // keeps track of the string of the specified column of the previous line
	buffer_pos := 0  // used to track the location in our byte array

	// Writing index file in 16MB chunks
	var b [16777216]byte
	for {
		buf, er := c.r.ReadLine()
		n := len(buf)
		if er != nil {
			if er != io.EOF {
				err = er
				return
			}
			eof = true
		}
		// skip empty line
		if n <= 1 {
			total_n += n
			line_count += 1
			if eof {
				break
			} else {
				continue
			}
		}
		// split line by columns and test if column value has changed
		slices := bytes.Split(buf, []byte("\t"))
		if len(slices) < column-1 {
			return 0, format, errors.New("Specified column does not exist for all lines in file.")
		}

		str := string(slices[column-1])
		if prev_str != str && line_count != 0 {
			// Calculating position in byte array
			x := (buffer_pos * 16)
			// Print byte array if it's full
			if x == 16777216 {
				f.Write(b[:])
				buffer_pos = 0
				x = 0
			}
			// Adding next record to byte array
			binary.LittleEndian.PutUint64(b[x:x+8], uint64(curr))
			binary.LittleEndian.PutUint64(b[x+8:x+16], uint64(total_n))

			curr += int64(total_n)
			count += 1
			buffer_pos += 1
			total_n = 0
			prev_str = str
		}
		if line_count == 0 {
			prev_str = str
		}
		total_n += n
		line_count += 1

		if eof {
			break
		}
	}

	// Calculating position in byte array
	x := (buffer_pos * 16)
	// Print byte array if it's full
	if x == 16777216 {
		f.Write(b[:])
		buffer_pos = 0
		x = 0
	}
	// Unless file was empty we need to add the last index to our byte array and then print it out
	if curr != 0 {
		binary.LittleEndian.PutUint64(b[x:x+8], uint64(curr))
		binary.LittleEndian.PutUint64(b[x+8:x+16], uint64(total_n))
		count += 1
		buffer_pos += 1
		f.Write(b[:buffer_pos*16])
	}

	err = os.Rename(tmpFilePath, ofile)

	return
}

func (c *column) Close() (err error) {
	c.f.Close()
	return
}
