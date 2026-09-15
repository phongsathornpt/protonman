package prompt

// AttachedImageInstruction is injected only for turns that already contain one
// or more native image content parts. It keeps image understanding on the
// multimodal model path instead of encouraging redundant shell/Python reads.
const AttachedImageInstruction = `# Attached Images
- Images attached to the current user turn are already available to the model as native multimodal input.
- Inspect and reason about attached images directly with the model's vision capability.
- Do not use read, bash, Python, OCR, or another tool merely to inspect an image that is already attached to the current turn.
- Use tools only for an explicit deterministic operation such as metadata inspection, exact pixel sampling, resize, crop, conversion, or when the image exists only as a workspace file and is not attached yet.
- Never convert an attached image into text through Python as a fallback for visual understanding.`
