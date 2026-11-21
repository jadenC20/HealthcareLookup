import sys
from pathlib import Path
import pytest
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))

@pytest.fixture(autouse=True)
def isolate_database(tmp_path,monkeypatch):
    import database
    db=str(tmp_path/'isolated.db')
    monkeypatch.setenv('CLEARCARE_DB',db)
    monkeypatch.setattr(database,'DB_PATH',db)
