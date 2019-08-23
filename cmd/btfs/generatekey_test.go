package main

import (
	"testing"
)

// func generateKey(importKey string, keyType string, seedPhrase string, mnemonicLen int, mnemonic string ) error {
//record
//access
// aerobic
// glow
// retreat
// language
// distance
// stamp
// cattle
// arrive
// defy
// movie
// TSZKYA3bd4sJEmthPk1Z9hfD5ijpaDM1QE
// db6bf5da1c2225d76356df2f59ebd3207b91228bcf429f0c791f94e9095a1f8e

func TestSeedsPhrase(t *testing.T) {
	const PRIVATE_KEY = "db6bf5da1c2225d76356df2f59ebd3207b91228bcf429f0c791f94e9095a1f8e"
	finalImportKey, err := generateKey(PRIVATE_KEY, "",
		"")
	if err != nil || finalImportKey != PRIVATE_KEY {
		t.Error("ImportKey generated not matching")
	}

	finalImportKey, err = generateKey("", "",
		"record,access,aerobic,glow,retreat,language,distance,stamp,cattle,arrive,defy,movie")
	if err != nil || finalImportKey != PRIVATE_KEY {
		t.Error("Generating from seed phrases failed")
	}

	// one less word
	finalImportKey, err = generateKey("", "",
		"record,access,aerobic,glow,retreat,language,distance,stamp,cattle,arrive,defy")
	if err.Error() != "The seed phrase required to generate TRON private key needs to contain 12 words. Provided mnemonic has 11 words." {
		t.Error("Parameter check failed")
	}
	// word not in dictionary
	finalImportKey, err = generateKey("", "",
		"record,access,aerobic,glow,retreat,language,distance,stamp,cattle,arrive,defy,mov")
	if err.Error() != "Entered seed phrase is not valid" {
		t.Error("Parameter check failed")
	}

}
