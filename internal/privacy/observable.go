package privacy

import "regexp"

var (
	unsafeMetadataAddressPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|account|acct[-_:./]|request[-_:./ ]?id|requestid|req[-_:./]|balance|credit|sse[-_:./ ]?chunk|tool[-_:./ ]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
	unsafePayloadMarkerPattern   = regexp.MustCompile(`(?i)(^|[/:._+ -])(raw([_:./ -](payload|body))?|payload|request[-_:./ ]?body|response[-_:./ ]?body|prompt[-_:./ ](text|body|payload)|completion[-_:./ ](text|body|payload))($|[/:._+ -])`)
	unsafeSnapshotStringPattern  = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|raw|payload|prompt|completion|body|account|acct[-_]|request[_ -]?id|requestid|req[-_]|balance|credit|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
	unsafeDisplayStringPattern   = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|raw|payload|prompt|completion|body|account|acct_|request[_ -]?id|requestid|req_|balance|credit|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
)

func UnsafeMetadataAddress(value string) bool {
	return unsafeMetadataAddressPattern.MatchString(value) || unsafePayloadMarkerPattern.MatchString(value)
}

func UnsafeSnapshotString(value string) bool {
	return unsafeSnapshotStringPattern.MatchString(value)
}

func UnsafeDisplayString(value string) bool {
	return unsafeDisplayStringPattern.MatchString(value)
}
