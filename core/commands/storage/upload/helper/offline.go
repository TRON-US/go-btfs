package helper

import (
	cmap "github.com/orcaman/concurrent-map"
)

var (
	EscrowChanMaps              = cmap.New()
	EscrowContractMaps          = cmap.New()
	GuardChanMaps               = cmap.New()
	GuardContractMaps           = cmap.New()
	BalanceChanMaps             = cmap.New()
	UnsignedChannelCommitMaps   = cmap.New()
	SignedChannelCommitChanMaps = cmap.New()
	PayinReqChanMaps            = cmap.New()
	FileMetaChanMaps            = cmap.New()
	QuestionsChanMaps           = cmap.New()
	WaitUploadChanMap           = cmap.New()
)
