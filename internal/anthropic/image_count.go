package anthropic

func RequestImageCount(req Request) int {
	count := 0
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if block.Type == "image" {
				count++
			}
		}
	}
	return count
}
