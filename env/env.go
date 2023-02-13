package env

import (
	"github.com/codingconcepts/env"
)

type Env struct {
	JobStorePath       string `env:"JOB_STORE_PATH" required:"true"`
	JobSSHPath         string `env:"JOB_SSH_PATH" required:"true"`
	JobTFTemplatesPath  string `env:"JOB_TF_TEMPLATES_PATH" required:"true"`

	StorePath        string `env:"STORE_PATH" required:"true"`
	SSHPath          string `env:"SSH_PATH" required:"true"`
	MySqlServiceName string `env:"CLUSTER_MYSQL_SERVICE_NAME" required:"true"`
}

func GetEnvOrDie() *Env {
	var ev Env
	if err := env.Set(&ev); err != nil {
		panic(err)
	}
	return &ev
}
