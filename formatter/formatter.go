package formatter

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/shared"
)

// IFormatter specifies what a Formatter does.
type IFormatter = processor.IProcessor

// mapBuilder is a helper function to build the JSON map.
//
//nolint:ifshort
func mapBuilder(m message.IMessage) map[string]interface{} {
	mM := map[string]interface{}{}

	mM["id"] = m.GetID()
	mM["contentBasedHashID"] = m.GetContentBasedHashID()
	mM["component"] = m.GetComponentName()
	mM["output"] = m.GetOutputName()
	mM["level"] = strings.ToLower(m.GetLevel().String())
	mM["timestamp"] = m.GetTimestamp().Format(time.RFC3339)
	mM["message"] = m.GetContent().GetProcessed()

	tags := m.GetTags()
	if len(tags) != 0 {
		mM["tags"] = tags
	}

	flg := m.GetFlag()
	if flg != flag.None {
		mM["flag"] = flg
	}

	outputsNames := m.GetOutputsNames()
	if len(outputsNames) != 0 {
		mM["outputsNames"] = outputsNames
	}

	processorsNames := m.GetProcessorsNames()
	if len(processorsNames) != 0 {
		mM["processorsNames"] = processorsNames
	}

	// Should only process fields if any.
	if len(m.GetFields()) != 0 {
		for k, v := range m.GetFields() {
			mM[k] = v
		}
	}

	return mM
}

//////
// Built-in processors.
//////

// JSONPretty is a prettified-JSON formatter. It automatically adds:
// - Component
// - Level
// - Message
// - Output
// - Tags
// - Timestamp (RFC3339).
// - Fields.
func JSONPretty() IFormatter {
	return processor.New("JSON", func(m message.IMessage) error {
		m.GetContent().SetProcessed(shared.Prettify(mapBuilder(m)))

		return nil
	})
}

// JSON is a JSON formatter. It automatically adds:
// - Component
// - Level
// - Message
// - Output
// - Tags
// - Timestamp (RFC3339).
// - Fields.
func JSON() IFormatter {
	return processor.New("JSON", func(m message.IMessage) error {
		m.GetContent().SetProcessed(shared.Inline(mapBuilder(m)))

		return nil
	})
}

// Text is a text formatter. It automatically adds:
// - Component
// - Level
// - Message
// - Output
// - Timestamp (RFC3339).
// - Fields.
func Text() IFormatter {
	return processor.New("Text", func(m message.IMessage) error {
		buf := new(strings.Builder)

		// Observe that the third line has no trailing tab,
		// so its final cell is not part of an aligned column.
		w := tabwriter.NewWriter(buf, 0, 0, 1, ' ', 0)

		fmt.Fprintf(w, "component=%s\t", m.GetComponentName())
		fmt.Fprintf(w, "output=%s\t", strings.ToLower(m.GetOutputName()))
		fmt.Fprintf(w, "level=%s\t", strings.ToLower(m.GetLevel().String()))
		fmt.Fprintf(w, "message=%s\t", m.GetContent().GetProcessed())
		fmt.Fprintf(w, "timestamp=%s\t", m.GetTimestamp().Format(time.RFC3339))

		// Should only process fields if any.
		if len(m.GetFields()) != 0 {
			for k, v := range m.GetFields() {
				fmt.Fprintf(w, "%s=%v\t", k, v)
			}
		}

		// Should only process fields if any.
		if len(m.GetTags()) != 0 {
			fmt.Fprintf(w, "tags=[%s]", strings.Join(m.GetTags(), ", "))
		}

		w.Flush()

		m.GetContent().SetProcessed(buf.String())

		return nil
	})
}
