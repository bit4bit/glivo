package dptools

import (
	"github.com/bit4bit/glivo"
	"bytes"
)

//Execute Bridge and returns the events hangup for the ALeg and BLeg of the call
func (dptools *DPTools) Bridge(endpoint string) (aleg *glivo.Event, bleg *glivo.Event) {
	var send bytes.Buffer

	send.WriteString(endpoint)


	
	wait_bridge := dptools.call.WaitExecute("bridge", map[string]string{
		"Event-Name": "CHANNEL_EXECUTE_COMPLETE",
		"Application": "bridge",
	})
	dptools.call.Execute("bridge", send.String(), true)
	<-wait_bridge


	wait_action := dptools.call.OnceEventHandle("bridge_action", func(res chan interface{}) glivo.HandlerEvent {
		return glivo.NewWaitAnyEventHandle(res,
			[]map[string]string{
				{"Event-Name": "CHANNEL_UNBRIDGE"},
				{"Event-Name": "CHANNEL_HANGUP"},
			})
	})

	action := (<-wait_action).(glivo.Event)


	switch action.Content["Event-Name"] {
		//hangup BLeg
	case "CHANNEL_UNBRIDGE":
		bleg = &action
		//wait hangup ALeg
		action_aleg := (<-wait_bridge).(glivo.Event)
		aleg = &action_aleg
		//hangup trying bridge
		//hangup ALeg
	case "CHANNEL_HANGUP":
		aleg = &action
	}
	
	return aleg, bleg
}
