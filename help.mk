# Helper file for Makefile to generate documentation for make targets.
# Cross-platform: works on Windows (PowerShell / cmd), macOS, and Linux.
# On Windows, if a POSIX shell is available (Git Bash / MSYS2), the Unix
# branch is used — otherwise PowerShell is invoked.

MAKEFILE_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))
ABSOLUT_MAKEFILE_PATH := $(MAKEFILE_DIR)Makefile

# On non-Windows systems, always use the Unix branch.
# On Windows, detect Git Bash / MSYS2 via the MSYSTEM env variable — both
# set it (MINGW64, MSYS, etc.) but native cmd/PowerShell do not. No subshell
# call required, so no stray files or stderr noise on either shell.
ifeq ($(OS),Windows_NT)
    ifeq ($(MSYSTEM),)
        DETECT_OS := Windows
    else
        DETECT_OS := Unix
    endif
else
    DETECT_OS := Unix
endif

.PHONY: help
help:
ifeq ($(DETECT_OS),Windows)
	@powershell -NoProfile -Command "Get-Content '$(ABSOLUT_MAKEFILE_PATH)' | ForEach-Object { if ($$_ -match '^([a-zA-Z0-9_-]+:).*##\s?(.+)$$') { '{0,-25} {1}' -f $$Matches[1], $$Matches[2] } }"
else
	@sed -ne 's/^\([^[:space:]]*\):.*##/\1: /p' $(ABSOLUT_MAKEFILE_PATH) | awk '{printf "%-25s %s\n", $$1, substr($$0, index($$0,$$2))}'
endif
