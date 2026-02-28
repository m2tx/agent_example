package assets

import "embed"

//go:embed *
var Dir embed.FS

//go:embed system_instruction.txt
var SystemInstruction string
