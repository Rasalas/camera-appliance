package releasearchive

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

var elfMachines = map[string]struct {
	machine elf.Machine
	class   elf.Class
}{
	"amd64":   {elf.EM_X86_64, elf.ELFCLASS64},
	"arm64":   {elf.EM_AARCH64, elf.ELFCLASS64},
	"386":     {elf.EM_386, elf.ELFCLASS32},
	"arm":     {elf.EM_ARM, elf.ELFCLASS32},
	"riscv64": {elf.EM_RISCV, elf.ELFCLASS64},
	"ppc64le": {elf.EM_PPC64, elf.ELFCLASS64},
	"s390x":   {elf.EM_S390, elf.ELFCLASS64},
	"loong64": {elf.EM_LOONGARCH, elf.ELFCLASS64},
}

// executableFormat names the executable container of header, or "" if unknown.
func executableFormat(header []byte) string {
	switch {
	case bytes.HasPrefix(header, []byte(elf.ELFMAG)):
		return "elf"
	case bytes.HasPrefix(header, []byte("MZ")):
		return "windows"
	case len(header) >= 4:
		switch binary.BigEndian.Uint32(header) {
		case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, 0xcafebabe:
			return "darwin"
		}
	}
	return ""
}

// checkReleaseBinary rejects a release CLI built for another OS or CPU, such as
// a darwin binary from `make release` on a Mac. Unknown formats pass.
func checkReleaseBinary(path, goos, goarch string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 64)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return err
	}
	if n == 0 {
		return fmt.Errorf("Release-Archiv ungültig: bin/camera-appliance ist leer")
	}
	expected := "elf"
	if goos == "darwin" || goos == "windows" {
		expected = goos
	}
	format := executableFormat(header[:n])
	if format == "" {
		return nil
	}
	if format != expected {
		return fmt.Errorf("Release-Archiv passt nicht zu diesem System: bin/camera-appliance ist ein %s-Programm, benötigt wird %s/%s", formatLabel(format), goos, goarch)
	}
	if format != "elf" {
		return nil
	}
	parsed, err := elf.NewFile(file)
	if err != nil {
		return fmt.Errorf("Release-Archiv ungültig: bin/camera-appliance ist kein lesbares ELF-Programm: %w", err)
	}
	defer parsed.Close()
	if goos == "linux" && parsed.OSABI != elf.ELFOSABI_NONE && parsed.OSABI != elf.ELFOSABI_LINUX {
		return fmt.Errorf("Release-Archiv passt nicht zu diesem System: bin/camera-appliance ist für %s gebaut, benötigt wird %s/%s", parsed.OSABI, goos, goarch)
	}
	if want, known := elfMachines[goarch]; known && (parsed.Machine != want.machine || parsed.Class != want.class) {
		return fmt.Errorf("Release-Archiv passt nicht zu diesem System: bin/camera-appliance ist für %s (%s) gebaut, benötigt wird %s/%s", parsed.Machine, parsed.Class, goos, goarch)
	}
	return nil
}

func formatLabel(format string) string {
	switch format {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return "Linux-ELF"
}
