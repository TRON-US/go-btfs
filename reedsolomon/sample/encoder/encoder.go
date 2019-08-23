package main

import (
	"flag"
	"fmt"
	"os"

	reedsol "github.com/tron-us/go-btfs/reedsolomon"

	"github.com/klauspost/reedsolomon"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  simple-encoder [-flags] filename.ext\n\n")
		fmt.Fprintf(os.Stderr, "Valid flags:\n")
		flag.PrintDefaults()
	}
}

func main() {
	// Parse command line parameters.
	dataShards, parShards, outDir, fname := reedsol.ParseCommandLine()

	//Validate reed-solomon config.
	isValid := reedsol.ValidateConfig(dataShards, parShards)
	if (!isValid) {
		fmt.Fprintf(os.Stderr, "Error: sum of data and parity shards cannot exceed 256\n")
		os.Exit(1)
	}

	// Create encoding matrix.
	enc, err := reedsolomon.New(*dataShards, *parShards)
	reedsol.CheckErr(err)

	// Load file and split the file into equally sized shards.
	shards:=reedsol.SplitFileData(enc, fname)

	// Generate parity shards
	err = enc.Encode(shards)
	reedsol.CheckErr(err)

	// Write out the resulting files.
	reedsol.WriteShardsToFiles(fname, outDir, shards)
}
