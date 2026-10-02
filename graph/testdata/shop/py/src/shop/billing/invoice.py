from . import tax
from .tax import rate
import os

def invoice():
    return tax.rate()
