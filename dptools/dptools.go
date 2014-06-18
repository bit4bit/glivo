package dptools

import (
	"github.com/bit4bit/glivo"
)

type DPTools struct {
	call *glivo.Call
}

func NewDPTools(call *glivo.Call) *DPTools  {
	return &DPTools{call: call}
}
