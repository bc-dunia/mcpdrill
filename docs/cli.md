# CLI Reference

Complete reference for the `mcpdrill` command-line tool.

## Commands

### Run Management

| Command | Description |
|---------|-------------|
| `mcpdrill create <config.json>` | Create a new test run from configuration |
| `mcpdrill start <run_id>` | Start a created run |
| `mcpdrill status <run_id>` | Get run status |
| `mcpdrill status <run_id> --json` | Get status as JSON |
| `mcpdrill stop <run_id>` | Graceful stop (drain in-flight requests) |
| `mcpdrill emergency-stop <run_id>` | Immediate termination |
| `mcpdrill events <run_id>` | Stream events once |
| `mcpdrill events <run_id> --follow` | Stream events continuously |
| `mcpdrill compare <run_a> <run_b>` | Compare metrics between two runs |

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--endpoint URL` | Control Plane API URL | `http://localhost:8080` |
| `--actor NAME` | Actor name for audit logging | `user` |

## Usage Examples

### Create and Run a Test

```bash
# Create a run and capture the ID
RUN_ID=$(./mcpdrill create config.json | grep -oE 'run_[0-9a-f]+')

# Start the run
./mcpdrill start $RUN_ID

# Monitor in real-time
./mcpdrill events $RUN_ID --follow
```

### Remote Control Plane

```bash
./mcpdrill --endpoint http://prod-server:8080 status run_0000000000000001
```

### Compare Runs

```bash
./mcpdrill compare run_0000000000000002 run_0000000000000003
```

### JSON Output for Scripting

```bash
./mcpdrill status run_0000000000000001 --json | jq '.state'
```

### Stop a Run

```bash
# Graceful stop (waits for in-flight requests)
./mcpdrill stop run_0000000000000001

# Emergency stop (immediate termination)
./mcpdrill emergency-stop run_0000000000000001
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Configuration error |
| 3 | Connection error |
