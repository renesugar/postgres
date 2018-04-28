package framework

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/appscode/kutil/tools/portforward"
	"github.com/go-xorm/xorm"
	"github.com/kubedb/postgres/pkg/controller"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) GetPostgresClient(meta metav1.ObjectMeta) (*xorm.Engine, error) {
	postgres, err := f.GetPostgres(meta)
	if err != nil {
		return nil, err
	}
	clientPodName := fmt.Sprintf("%v-0", postgres.Name)
	tunnel := portforward.NewTunnel(
		f.kubeClient.CoreV1().RESTClient(),
		f.restConfig,
		postgres.Namespace,
		clientPodName,
		controller.PostgresPort,
	)
	if err := tunnel.ForwardPort(); err != nil {
		return nil, err
	}

	cnnstr := fmt.Sprintf("user=postgres host=127.0.0.1 port=%v dbname=postgres sslmode=disable", tunnel.Local)
	return xorm.NewEngine("postgres", cnnstr)
}

func (f *Framework) EventuallyCreateSchema(meta metav1.ObjectMeta) GomegaAsyncAssertion {

	sql := `
DROP SCHEMA IF EXISTS "data" CASCADE;
CREATE SCHEMA "data" AUTHORIZATION "postgres";
`
	return Eventually(
		func() bool {
			db, err := f.GetPostgresClient(meta)
			if err != nil {
				return false
			}

			if err := f.CheckPostgres(db); err != nil {
				return false
			}

			_, err = db.Exec(sql)
			if err != nil {
				return false
			}
			return true
		},
		time.Minute*5,
		time.Second*5,
	)
}

var randChars = []rune("abcdefghijklmnopqrstuvwxyzabcdef")

// Use this for generating random pat of a ID. Do not use this for generating short passwords or secrets.
func characters(len int) string {
	bytes := make([]byte, len)
	rand.Read(bytes)
	r := make([]rune, len)
	for i, b := range bytes {
		r[i] = randChars[b>>3]
	}
	return string(r)
}

func (f *Framework) EventuallyPingDatabase(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			db, err := f.GetPostgresClient(meta)
			if err != nil {
				return false
			}

			if err := f.CheckPostgres(db); err != nil {
				return false
			}

			return true
		},
		time.Minute*10,
		time.Second*5,
	)
}

func (f *Framework) EventuallyCreateTable(meta metav1.ObjectMeta, total int) GomegaAsyncAssertion {
	count := 0
	return Eventually(
		func() bool {
			db, err := f.GetPostgresClient(meta)
			if err != nil {
				return false
			}

			if err := f.CheckPostgres(db); err != nil {
				return false
			}

			for i := count; i < total; i++ {
				table := fmt.Sprintf("SET search_path TO \"data\"; CREATE TABLE %v ( id bigserial )", characters(5))
				_, err := db.Exec(table)
				if err != nil {
					return false
				}
				count++
			}
			return true
		},
		time.Minute*5,
		time.Second*5,
	)

	return nil
}

func (f *Framework) EventuallyCountTable(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() int {
			db, err := f.GetPostgresClient(meta)
			if err != nil {
				return -1
			}

			if err := f.CheckPostgres(db); err != nil {
				return -1
			}

			res, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema='data'")
			if err != nil {
				return -1
			}

			return len(res)
		},
		time.Minute*10,
		time.Second*5,
	)
}

func (f *Framework) CheckPostgres(db *xorm.Engine) error {
	err := db.Ping()
	if err != nil {
		return err
	}
	return nil
}

type PgStatArchiver struct {
	ArchivedCount int
}

func (f *Framework) EventuallyCountArchive(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	previousCount := -1
	countSet := false
	return Eventually(
		func() bool {
			db, err := f.GetPostgresClient(meta)
			if err != nil {
				return false
			}

			if err := f.CheckPostgres(db); err != nil {
				return false
			}

			var archiver PgStatArchiver
			if _, err := db.Limit(1).Cols("archived_count").Get(&archiver); err != nil {
				return false
			}

			if !countSet {
				countSet = true
				previousCount = archiver.ArchivedCount
				return false
			} else {
				if archiver.ArchivedCount > previousCount {
					return true
				}
			}
			return false
		},
		time.Minute*5,
		time.Second*5,
	)
}
