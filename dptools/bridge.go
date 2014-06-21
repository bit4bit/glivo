package dptools

import (
	"github.com/bit4bit/glivo"
	"bytes"
)

//Execute Bridge and returns the events hangup for the ALeg and BLeg of the call
func (dptools *DPTools) Bridge(endpoint string) *glivo.Event {
	var send bytes.Buffer

	send.WriteString(endpoint)


	wait_action := dptools.call.OnceEventHandle("bridge_action", func(res chan interface{}) glivo.HandlerEvent {
		return glivo.NewWaitAnyEventHandle(res,
			[]map[string]string{
				{"Event-Name": "CHANNEL_UNBRIDGE"},
				{"Event-Name": "CHANNEL_HANGUP"},
			})
	})

        wait_complete := dptools.call.WaitExecute("bridge", map[string]string{
                "Event-Name": "CHANNEL_EXECUTE_COMPLETE",
                "Application": "bridge",
        })

	dptools.call.Execute("bridge", send.String(), true)

	action := (<-wait_action).(glivo.Event)

	<-wait_complete

	return &action
}
