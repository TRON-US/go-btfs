package spin

import (
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/upload"

	cmds "github.com/TRON-US/go-btfs-cmds"

	"go4.org/syncutil"
)

func RenterSessions(req *cmds.Request, env cmds.Environment) {
	go func() {
		params, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return
		}

		cursor, err := sessions.GetRenterSessionsCursor(params)
		if err != nil {
			return
		}
		sem := syncutil.NewSem(10)
		session, err := cursor.NextSession(sessions.RssWaitUploadReqSignedStatus)
		for err == nil && session != nil {
			go func(session *sessions.RenterSession) {
				sem.Acquire(1)
				defer sem.Release(1)
				if e := upload.ResumeWaitUploadOnSigning(session); e != nil {
					return
				}
			}(session)
		}
	}()
}
