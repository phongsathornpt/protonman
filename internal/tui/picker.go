package tui

func normalizedPickerWindow(index, offset, count, visible int) (int, int, int) {
	if count <= 0 {
		return 0, 0, 0
	}
	if visible <= 0 {
		visible = 1
	}
	if index < 0 {
		index = 0
	} else if index >= count {
		index = count - 1
	}
	if offset < 0 {
		offset = 0
	}
	if index < offset {
		offset = index
	} else if index >= offset+visible {
		offset = index - visible + 1
	}
	maxOffset := count - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	end := offset + visible
	if end > count {
		end = count
	}
	return index, offset, end
}
