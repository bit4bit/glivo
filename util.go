package glivo

import (
	"net/textproto"
	"net/url"

)

func mimeToMap(mime textproto.MIMEHeader) (ret map[string]string) {
	ret = make(map[string]string)
	for k,v := range mime {
		val,err := url.QueryUnescape(v[0])
		if err != nil {
			val = v[0]
		}
		ret[k] = val
	}

	return
}

