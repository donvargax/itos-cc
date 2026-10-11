"""coverage.py before 7.14, as far as its reports go.

Before 7.14, a reporting command (coverage lcov, coverage report) read its
data file alone; 7.14 made it combine the parallel data files beside it
first. A .pth file in a test project's .venv, which sees the system's
coverage.py, imports this module as Python starts, before coverage.py loads,
and once coverage.control loads, its Coverage.combine does nothing when
called as those commands call it, with no paths and not strict. A combine
given its paths, as coverage combine and itos-cc's scripts call it, runs.
"""

import importlib.machinery
import sys


class ReportsCombineNothing:
    def find_spec(self, name, path=None, target=None):
        if name != "coverage.control":
            return None
        sys.meta_path.remove(self)
        spec = importlib.machinery.PathFinder.find_spec(name, path)
        load = spec.loader.exec_module

        def exec_module(module):
            load(module)
            combine = module.Coverage.combine

            def reporting(self, data_paths=None, strict=False, keep=False):
                if data_paths is None and not strict:
                    return None
                return combine(self, data_paths, strict, keep)

            module.Coverage.combine = reporting

        spec.loader.exec_module = exec_module
        return spec


sys.meta_path.insert(0, ReportsCombineNothing())
