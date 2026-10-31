from typing import Callable

from policies.contract import (
    PodRequest,
    CandidateServer,
    RankedServer,
)

PolicyFunction = Callable[
    [PodRequest, list[CandidateServer]],
    list[RankedServer],
]

POLICIES: dict[str, PolicyFunction] = {}


def register(name: str):
    """Register a scheduling policy by its unique name."""

    def decorator(policy: PolicyFunction):
        if name in POLICIES:
            raise ValueError(f"Policy already registered: {name}")

        POLICIES[name] = policy
        return policy

    return decorator


def get_policy(name: str) -> PolicyFunction:
    """Return a registered policy."""

    if name not in POLICIES:
        raise ValueError(
            f"Unknown policy: {name}. Available: {list_policies()}"
        )

    return POLICIES[name]


def list_policies() -> list[str]:
    return sorted(POLICIES.keys())
