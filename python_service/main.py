
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from policies.energy_aware import score_nodes

app = FastAPI(title="EAWSK Python Policy Service")


class ScoreRequest(BaseModel):
    nodes: list[dict[str, Any]]


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/score")
def score(request: ScoreRequest):
    try:
        return {"nodes": score_nodes(request.nodes)}
    except (KeyError, TypeError, ValueError) as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
