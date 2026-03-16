# CSX - Command Security Explorer

The fully compiled, pure-Golang interactive command-line security tool.

CSX provides an instantaneous, searchable TUI (Terminal User Interface) overlay built upon a customizable JSON database of Active Directory, Privilege Escalation, Web Application, and Network auditing commands.

## Architecture & Benefits

This project is built from the ground up using Go and **Charmbracelet's Bubble Tea** rendering suite.
*   **Zero Dependencies:** No `jq`, `awk`, `grep`, `sed`, or `fzf` required.
*   **Run Anywhere:** Because CSX is pre-compiled, you can effortlessly run it on raw VMs, isolated Pentest dropboxes, or stripped-down alpine containers precisely as is. 
*   **Memory Native:** Parses the 230+ JSON command tree in sub-milliseconds without relying on spawned bash sub-shells.

## Installation (Virtual Machine / Global Deploy)

Because CSX is a compiled binary, you only need to copy two files to any target VM to deploy it globally: `csx` (the binary) and `commands.json` (the database).

### 1. The Recommended Deployment Pathway
To safely install and use the tool on any VM, place the directory securely in `/opt/`, then symlink the executable to the system binary path cleanly.

```bash
# 1. Download or transfer the 'csx-go' directory to your VM. Let's assume it's currently in your Downloads folder.

# 2. Move the directory to a global application path
sudo mv ~/Downloads/csx-go /opt/csx

# 3. Create a symbolic link allowing execution from anywhere
sudo ln -s /opt/csx/csx /usr/local/bin/csx

# 4. If necessary, ensure it has execution permissions
sudo chmod +x /usr/local/bin/csx
```

*Note: CSX dynamically maps its own binary location using `os.Executable()`. Creating a symlink natively tells it to always check `/opt/csx/` for its JSON database, meaning it will never crash.*

## UI Hotkeys

CSX features a custom-built, native Bubble Tea Table overlaid with instant Category sorting.

### Navigation
- `Arrow Up` / `Arrow Down` : Scroll Commands
- `Enter` : Select Command (Extracts Variables)
- `Esc` : Return / Quit

### Phase Filtering
Immediately sort the database using the following native keybinds:
- `ctrl-a`: **All Commands** (Default)
- `ctrl-r`: **Recon** Phase
- `ctrl-w`: **Web Apps**, HTTP, URL Fuzzing
- `ctrl-p`: Local **PrivEsc**
- `ctrl-c`: **Cloud**, Docker, Kubernetes
- `ctrl-t`: **Thick Client** & Mobile Auditing
- `ctrl-l`: **Linux** Core System Maintenance

## Customizing The Database

You can natively append, delete, or rewrite any entries inside `commands.json`. 

The core feature of CSX is **Regex Variable Interpolation**. When writing your custom commands, bracket any required variable precisely as `{variableString}`.

Example:
```json
{
  "tool": "Nmap Udp",
  "description": "Standard heavy aggressive UDP scanner.",
  "phase": "recon",
  "command": "sudo nmap -sU --top-ports 100 -oA {output_file} {target_ip}"
}
```
When you press `Enter` on this tool in the UI, CSX will dynamically map an explicit array of `textinputs` requiring the user to fill out exactly two parameters (`output_file` and `target_ip`), ensuring command templates never fail from typos.

## Build From Source / Cross-Compilation

If you plan to modify `main.go` and need to natively rebuild the binary, Go makes it incredibly easy to compile for different operating systems without requiring a VM.

### Standard Build
```bash
go mod tidy
go build -ldflags="-s -w" -o csx main.go
```
*(The `-s -w` flags strip the DWARF debugging tables, shrinking the binary significantly!)*

### Building and Installing Natively on Linux
If you are modifying the code directly inside your Linux/WSL VM, you can build it and deploy the symlink in one action:
```bash
cd /opt/csx-go/
go build -o csx main.go
sudo chmod +x csx
sudo ln -s /opt/csx-go/csx /usr/local/bin/csx-go
```

### Cross-Compiling from Windows to Linux
If you are modifying the code on a **Windows** host but need to deploy the finished tool to an Ubuntu or Kali VM:
```bash
# In PowerShell:
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -ldflags="-s -w" -o csx main.go
```
This produces a monolithic, dependency-free Linux executable that you can drop straight into `/opt/`!
