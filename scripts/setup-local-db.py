"""Create an isolated local development database inside WSL. No credential output."""
from pathlib import Path
import secrets
import subprocess

root = Path(__file__).resolve().parents[1]
runtime = root / '.runtime'
runtime.mkdir(exist_ok=True)
env = runtime / 'server.env'
if not env.exists():
    password = secrets.token_hex(24)
    sql = "CREATE ROLE aihub_m0 LOGIN PASSWORD '" + password + "';\nCREATE DATABASE aihub_m0 OWNER aihub_m0;"
    subprocess.run(['runuser', '-u', 'postgres', '--', 'psql', '-v', 'ON_ERROR_STOP=1'], input=sql, text=True, check=True, stdout=subprocess.DEVNULL)
    url = 'postgres://aihub_m0:' + password + '@127.0.0.1:5432/aihub_m0?sslmode=disable'
    env.write_text('export AIHUB_DATABASE_URL="'+url+'"\nexport AIHUB_TEST_DATABASE_URL="'+url+'"\n')
    env.chmod(0o600)
print('Local PostgreSQL database is ready; credentials stored in .runtime/server.env')
