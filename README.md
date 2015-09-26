glivo
=====

Framework for building interactive telephony -Freeswitch- apps on Go Lang


glivo es
========

*glivo* es un framework para la construccion de applicaciones de telefonia en Freeswitch.

*NO ACTO PARA PRODUCCION, AUN*

Uso
---
Para ejemplos mirar: https://github.com/bit4bit/glivo-examples


TODO
---
 * temporizadores en los eventos y generacion de errores
 * Pruebas de comportamiento
 * Compatible con PlivoFramework
 

# EJEMPLOS

~~~go
func HandleCall(call *glivo.Call, data interface{}) {
	call.WaitAnswer()
	call.Answer()
	.....
	call.Hangup()
}

//iniciar glivo en el puerto e interfaz deseada
fsout, err := glivo.Listen(*clientIP, logger)
if err != nil {
	logger.Fatal(err.Error())
	os.Exit(1)
}
defer func() {
	fsout.Stop()
}()
go fsout.Serve(HandleCall, nil)
~~~

o para un manejador
~~~go
func HandleCall(call *glivo.Call, data interface{}) {
	call.WaitAnswer()
	call.Answer()
	.....
	call.Hangup()
}

//iniciar glivo en el puerto e interfaz deseada
glivo.ListenAndServe(*clientIP, logger, HandleCall, nil)
~~~