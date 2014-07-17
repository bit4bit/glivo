package chain

import (
	"fmt"
	"github.com/bit4bit/glivo"
	"strconv"
	//	"os"
)

//Permite Concatenar acciones alrededor del GetDigits
//Esto simula el GetDigits de *plivoframework* :)
//Ej:
//chain := NewChainDigits(call)
//chain.SetTimeout(10)
//chain.Play("mi audio").Play("Then other.wav").Reply() //get response
type ChainDigits struct {
	call               *glivo.Call
	timeout            uint16
	digitTimeout       uint16
	finishOnKey        uint8
	numDigits          int
	retries            uint8
	playBeep           bool
	validDigits        string
	invalidDigitsSound string

	commands []CommandChainable
	result   chan glivo.Event
}

func NewChainDigits(call *glivo.Call) *ChainDigits {
	return &ChainDigits{
		call:               call,
		timeout:            5,
		digitTimeout:       2,
		finishOnKey:        '#',
		numDigits:          99,
		retries:            1,
		playBeep:           false,
		validDigits:        "123456789*#",
		invalidDigitsSound: "silence_stream://250",
		commands:           make([]CommandChainable, 50),
		result:             make(chan glivo.Event),
	}
}

type ChainableDigits interface {
	Play(files string) ChainDigits
	Speak(phrase string) ChainDigits
	Wait(seconds int) ChainDigits
}

func (digits *ChainDigits) SetTimeout(t uint16) {
	digits.timeout = t
}

func (digits *ChainDigits) SetDigitTimeout(t uint16) {
	digits.digitTimeout = t
}

func (digits *ChainDigits) SetFinishOnKey(k uint8) {
	digits.finishOnKey = k
}

func (digits *ChainDigits) SetNumDigits(n int) {
	digits.numDigits = n
}

func (digits *ChainDigits) SetRetries(n uint8) {
	digits.retries = n
}

func (digits *ChainDigits) SetPlayBeep(b bool) {
	digits.playBeep = b
}

func (digits *ChainDigits) SetInvalidDigitsSound(s string) {
	digits.invalidDigitsSound = s
}

func (digits *ChainDigits) SetValidDigits(s string) {
	digits.validDigits = s
}

//Reproduce archivo local al servidor Freeswitch
func (digits *ChainDigits) Play(file string) *ChainDigits {
	digits.commands = append(digits.commands, CommandChainable{"play", file})
	return digits
}

func (digits *ChainDigits) Speak(phrase string) *ChainDigits {
	digits.commands = append(digits.commands, CommandChainable{"say", phrase})
	return digits
}

func (digits *ChainDigits) Wait(seconds int) *ChainDigits {
	digits.commands = append(digits.commands, CommandChainable{"wait", strconv.Itoa(seconds)})
	return digits
}

//Espera que se responda con los digitos indicados
//y retorna si o no
func (digits *ChainDigits) Question(question string) (bool, error) {
	defer func() {
		digits.commands = nil
	}()

	separator := "!"

	outputs := make([]string, 200)

	for _, command := range digits.commands {
		switch command.app {
		case "say":
			//@todo permitir cambiar Engine y Voz
			outputs = append(outputs, fmt.Sprintf("say:flite:slt:%s", command.args))
		case "wait":
			value, err := strconv.Atoi(command.args)
			if err != nil {
				value = 1
			}
			outputs = append(outputs, fmt.Sprintf("file_string://silence_stream://%d", value*1000))
		case "play":
			outputs = append(outputs, command.args)

		}
	}

	sound_file := ""
	for _, output := range outputs {
		if len(output) == 0 {
			continue
		}
		if len(sound_file) == 0 {
			sound_file += output
		} else {
			sound_file += separator + output
		}
	}

	regexp := "^("
	regexp += question
	regexp += ")$"

	cmd := fmt.Sprintf("%d %d %d %d '%c' '%s' %s pagd_input %s %d",
		len(question),
		len(question),
		digits.retries,
		digits.timeout*1000,
		digits.finishOnKey,
		sound_file,
		digits.invalidDigitsSound,
		regexp,
		digits.digitTimeout*1000,
	)

	block := make(chan interface{})
	digits.call.RegisterEventHandle("getdigits_app",
		glivo.NewWaitEventHandle(block, map[string]string{
			"Variable_read_result": "success",
			"Application":          "play_and_get_digits",
		}),
	)

	digits.call.SetVar("playback_delimiter", "!")
	digits.call.SetVar("playback_terminators", "none")
	digits.call.Execute("play_and_get_digits", cmd, true)
	digits.call.Reply()

	digits.commands = nil

	ev := (<-block).(glivo.Event)
	digits.call.UnregisterEventHandle("getdigits_app")
	return ev.Content["Variable_pagd_input"] == question, nil
}

//Espera digitos, y colecion al final retonar
//toda la secuencia digitada
func (digits *ChainDigits) CollectInput() (string, error) {
	defer func() {
		digits.commands = nil
	}()

	separator := "!"

	outputs := make([]string, 200)

	for _, command := range digits.commands {
		switch command.app {
		case "say":
			//@todo permitir cambiar el engine y la voz
			outputs = append(outputs, fmt.Sprintf("say:flite:slt:%s", command.args))
		case "wait":
			value, err := strconv.Atoi(command.args)
			if err != nil {
				value = 1
			}
			outputs = append(outputs, fmt.Sprintf("file_string://silence_stream://%d", strconv.Itoa(value*1000)))
		case "play":
			outputs = append(outputs, command.args)

		}
	}

	sound_file := ""
	for _, output := range outputs {
		if len(output) == 0 {
			continue
		}
		if len(sound_file) == 0 {
			sound_file += output
		} else {
			sound_file += separator + output
		}
	}

	regexp := "^("
	for _, c := range digits.validDigits {
		if len(regexp) == 2 {
			regexp += fmt.Sprintf("%c", c)
		} else {
			if c >= '0' && c <= '9' {
				regexp += fmt.Sprintf("|%c", c)
			} else {
				regexp += fmt.Sprintf("|\\%c", c)
			}
		}
	}
	regexp += ")+$"

	cmd := fmt.Sprintf("%d %d %d %d '%c' '%s' %s pagd_input %s %d",
		1,
		digits.numDigits,
		digits.retries,
		digits.timeout*1000,
		digits.finishOnKey,
		sound_file,
		digits.invalidDigitsSound,
		regexp,
		digits.digitTimeout*1000,
	)

	block := make(chan string)
	cldtmf := glivo.NewCollectDTMFEventHandle(block, digits.numDigits, digits.validDigits, digits.finishOnKey)
	uuid := digits.call.RegisterEventHandleUUID(cldtmf)

	digits.call.SetVar("playback_delimiter", "!")
	digits.call.SetVar("playback_terminators", "none")
	digits.call.Execute("play_and_get_digits", cmd, true)
	digits.call.Reply()

	digits.commands = nil

	collection := <-block
	digits.call.UnregisterEventHandle(uuid)
	return collection, nil
}

//Espera digitos, y colecion al final retonar
//toda la secuencia digitada
func (digits *ChainDigits) Do() {
	defer func() {
		digits.commands = nil
	}()

	separator := "!"

	outputs := make([]string, 200)

	for _, command := range digits.commands {
		switch command.app {
		case "say":
			//@todo permitir cambiar el engine y la voz
			outputs = append(outputs, fmt.Sprintf("say:flite:slt:%s", command.args))
		case "wait":
			value, err := strconv.Atoi(command.args)
			if err != nil {
				value = 1
			}
			outputs = append(outputs, fmt.Sprintf("file_string://silence_stream://%d", strconv.Itoa(value*1000)))
		case "play":
			outputs = append(outputs, command.args)

		}
	}

	sound_file := ""
	for _, output := range outputs {
		if len(output) == 0 {
			continue
		}
		if len(sound_file) == 0 {
			sound_file += output
		} else {
			sound_file += separator + output
		}
	}
	digits.call.SetVar("playback_delimiter", "!")
	digits.call.SetVar("playback_terminators", "none")
	digits.call.Execute("playback", sound_file, true)
	digits.call.Reply()
}
