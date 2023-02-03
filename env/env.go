package env

import (
	"github.com/codingconcepts/env"
)

type Env struct {
	StorePath       string `env:"STORE_PATH" required:"true"`
	SSHPath         string `env:"SSH_PATH" required:"true"`
	TFTemplatesPath string `env:"TF_TEMPLATES_PATH" required:"true"`
}

func GetEnvOrDie() *Env {
	var ev Env
	if err := env.Set(&ev); err != nil {
		panic(err)
	}
	return &ev
}
