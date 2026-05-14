import jax
import jax.numpy as jnp


def pairwise_scores(x, w):
    """Deliberately wasteful JAX-style code for the optimization-loop example."""
    rows = []
    for i in range(x.shape[0]):
        weighted = x[i] * w
        rows.append(jnp.sum(jnp.sin(weighted) + jnp.cos(weighted * weighted)))
    stacked = jnp.stack(rows)
    print("debug score shape", stacked.shape)
    return stacked


def loss(x, w):
    scores = pairwise_scores(x, w)
    return jnp.mean(scores)
