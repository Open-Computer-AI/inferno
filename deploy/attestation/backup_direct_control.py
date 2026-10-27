"""Candidate-only case control with an acknowledgement from the AWS actor."""
import json
import pathlib
import tempfile
import time

from backup_direct_transport import parse_control
from backup_helpers import owned_file


def apply_case(root, case):
    private = root / 'private'
    pointer = private / 'aws-latency-active.json'
    owned_file(pointer, 0o600)
    state = json.loads(pointer.read_text())
    path = pathlib.Path(state['metadata']).resolve()
    if path.parent != root or path.name != 'aws-latency-processes-' + state['run_id'] + '.json':
        raise ValueError('unexpected direct actor metadata')
    owned_file(path, 0o600)
    metadata = json.loads(path.read_text())
    phase = json.loads((private / 'live-manifest.json').read_text())['run_id']
    if not metadata.get('direct_other') or not metadata.get('serve') or metadata.get('phase_run_id') != phase:
        raise ValueError('direct actor does not belong to this Live phase')
    record_path = pathlib.Path(metadata['records']).resolve()
    if record_path.parent != root or record_path.name != 'aws-latency-records-' + state['run_id'] + '.jsonl':
        raise ValueError('unexpected direct actor records')
    owned_file(record_path, 0o600)
    control = {'case': case, 'sequence': time.time_ns()}
    parse_control(json.dumps(control))
    target = private / 'aws-direct-control.json'
    if target.exists():owned_file(target, 0o600)
    with tempfile.NamedTemporaryFile(mode='w', dir=private, prefix='direct-control-', delete=False) as stream:
        json.dump(control, stream)
        temporary = pathlib.Path(stream.name)
    temporary.replace(target)
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        for line in reversed(record_path.read_text().splitlines()):
            try:row = json.loads(line)
            except json.JSONDecodeError:continue
            if row.get('aws_control_failed'):
                raise RuntimeError('direct actor rejected control')
            if row.get('aws_direct_control_applied') and row.get('sequence') == control['sequence']:
                if row.get('case') != case or row.get('run_id') != state['run_id']:
                    raise RuntimeError('direct control acknowledgement differs')
                down = case in {'direct-tunnel-down-air', 'direct-two-down-this', 'direct-all-down'}
                if row['direct_ssh_alive'] == down or row['direct_port_open'] == down:
                    raise RuntimeError('direct control transport state differs')
                return row
        time.sleep(.1)
    raise RuntimeError('direct control acknowledgement unavailable')
