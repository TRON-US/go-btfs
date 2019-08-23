package reedsolomon

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/klauspost/reedsolomon"
)

func ParseCommandLine() (*int, *int, *string, string) {
	var dataShards = flag.Int("data", 4, "Number of shards to split the data into")
	var parShards = flag.Int("par", 2, "Number of parity shards")
	var out = flag.String("out", "", "Alternative output")

	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Error: No filename given\n")
		flag.Usage()
		os.Exit(1)
	}
	fname := args[0]

	return dataShards, parShards, out, fname
}

func ValidateConfig(dataShards *int, parShards *int) bool {
	if (*dataShards + *parShards) > 256 {
		return false
	}
	return true
}

func CreateShardsFromData(dataShards *int, parShards *int, fname string) [][]byte {
	shards := make([][]byte, *dataShards+*parShards)
	for i := range shards {
		infn := fmt.Sprintf("%s.%d", fname, i)
		fmt.Println("Opening", infn)
		var err error
		shards[i], err = ioutil.ReadFile(infn)
		if err != nil {
			fmt.Println("Error reading file", err)
			shards[i] = nil
		}
	}
	return shards
}

func VerifyShards(shards [][]byte, enc reedsolomon.Encoder) {
	ok, err := enc.Verify(shards)
	if ok {
		fmt.Println("No reconstruction needed")
	} else {
		fmt.Println("Verification failed. Reconstructing data")
		err = enc.Reconstruct(shards)
		if err != nil {
			fmt.Println("Reconstruct failed -", err)
			os.Exit(1)
		}
		ok, err = enc.Verify(shards)
		if !ok {
			fmt.Println("Verification failed after reconstruction, data likely corrupted.")
			os.Exit(1)
		}
		CheckErr(err)
	}
}

func SplitFileData(enc reedsolomon.Encoder, fname string) [][]byte {
	fmt.Println("Opening", fname)
	data, err := ioutil.ReadFile(fname)
	CheckErr(err)

	shards, err := enc.Split(data)
	CheckErr(err)
	fmt.Printf("File split into %d data+parity shards with %d bytes/shard.\n", len(shards), len(shards[0]))
	return shards

}

func WriteShardsToFiles(fname string, outDir *string, shards [][]byte) {
	dir, file := filepath.Split(fname)
	if *outDir != "" {
		dir = *outDir
	}
	for i, shard := range shards {
		outfn := fmt.Sprintf("%s.%d", file, i)

		fmt.Println("Writing to", outfn)
		err := ioutil.WriteFile(filepath.Join(dir, outfn), shard, 0644)
		CheckErr(err)
	}
}

func CheckErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(2)
	}
}
