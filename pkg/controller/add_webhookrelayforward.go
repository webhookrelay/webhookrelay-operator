package controller

import (
	"github.com/webhookrelay/webhookrelay-operator/pkg/controller/webhookrelayforward"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, webhookrelayforward.Add)
}
