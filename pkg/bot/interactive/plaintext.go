package interactive

import "github.com/kubeshop/botkube/pkg/api"

// MessageToPlaintext returns interactive message as a plaintext.
func MessageToPlaintext(msg api.Message, newlineFormatter func(in string) string) string {
	msg.Description = ""

	fmt := MDFormatter{
		newlineFormatter:           newlineFormatter,
		headerFormatter:            NoFormatting,
		codeBlockFormatter:         NoFormatting,
		adaptiveCodeBlockFormatter: NoFormatting,
	}
	return RenderMessage(fmt, msg)
}
