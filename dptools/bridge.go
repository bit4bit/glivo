package dptools

import (
	"github.com/bit4bit/glivo"
	"bytes"
)

//Execute Bridge and returns the events hangup for the ALeg and BLeg of the call
func (dptools *DPTools) Bridge(endpoint string) (aleg *glivo.Event, bleg *glivo.Event) {
	var send bytes.Buffer

	send.WriteString(endpoint)

	wait_bridge := make(chan glivo.Event)
	dptools.call.RegisterEventHandle("bridge_action",
		glivo.NewWaitEventHandle(wait_bridge, map[string]string{
			"Event-Name": "CHANNEL_EXECUTE",
			"Application": "bridge",
		}),
	)
	

	dptools.call.Execute("bridge", send.String(), true)
	<-wait_bridge
	dptools.call.UnregisterEventHandle("bridge_action")


	dptools.call.RegisterEventHandle("bridge_action",
		glivo.NewWaitAnyEventHandle(wait_bridge,
			[]map[string]string{
				{"Event-Name": "CHANNEL_UNBRIDGE"},
				{"Event-Name": "CHANNEL_HANGUP"},
			}),
	)

	action := <-wait_bridge



	switch action.Content["Event-Name"] {
		//hangup BLeg
	case "CHANNEL_UNBRIDGE":
		bleg = &action
		//wait hangup ALeg
		action_aleg := <-wait_bridge
		aleg = &action_aleg
		//hangup trying bridge
		//hangup ALeg
	case "CHANNEL_HANGUP":
		aleg = &action
	}
	
	return aleg, bleg
}
