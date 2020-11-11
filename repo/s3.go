package repo

import (
	"bytes"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	logging "github.com/ipfs/go-log"
)

type S3Connection struct {
	role     string
	region   string
	s3bucket string
	folder   string
}

var DefaultS3Connection = &S3Connection{
	role:     "arn:aws:iam::213532986755:role/OrganizationAccountAccessRole",
	region:   "us-east-1",
	s3bucket: "btfs-dev.bt.co",
	folder:   "guard_test/",
}

var log = logging.Logger("s3")

func (s *S3Connection) getSessionService() *s3.S3 {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(s.region),
	})
	if err != nil {
		log.Errorf("Failed to create new session with aws config, reasons: [%v]", err)
	}
	return s3.New(sess)
}

func (s *S3Connection) RetrieveData(Key string) (raw []byte, err error) {
	return s.retrieveDataAbstractPath(s.folder + Key)
}

func (s *S3Connection) retrieveDataAbstractPath(directFile string) (raw []byte, err error) {
	out, err := s.getSessionService().GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.s3bucket),
		Key:    aws.String(directFile),
	})

	if err != nil {
		if strings.HasPrefix(err.Error(), "NoSuchKey") {
			return []byte(""), nil
		}
		return nil, err
	}

	buf := new(bytes.Buffer)
	if _, err = buf.ReadFrom(out.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
