"""The package as it will be published: its metadata, its typing marker, its imports, and what its README says."""

import ast
import importlib
import inspect
import re
import sys
import unittest
from pathlib import Path
from types import ModuleType
from typing import Any, Optional

import carragent
import carragent.client
import carragent.errors
import carragent.models

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "src" / "carragent"
KIT = ROOT.parent  # the agent kit's root, when this folder sits inside it (it does not in a source distribution)

# What a client of this package may import when `sys.stdlib_module_names` (Python 3.10 and later) is not there to ask.
STDLIB = {
    "base64",
    "collections",
    "datetime",
    "email",
    "http",
    "ipaddress",
    "json",
    "math",
    "re",
    "socket",
    "time",
    "typing",
    "urllib",
}

# typing names that Python 3.9 does not have.
NEWER_TYPING = {
    "Self",
    "TypeAlias",
    "ParamSpec",
    "Concatenate",
    "TypeGuard",
    "TypeIs",
    "Required",
    "NotRequired",
    "ReadOnly",
    "LiteralString",
    "Unpack",
    "TypeVarTuple",
    "Never",
    "assert_never",
    "assert_type",
    "reveal_type",
    "dataclass_transform",
    "override",
}


def _toml(path: Path) -> Optional[dict[str, Any]]:
    try:
        tomllib: Any = importlib.import_module("tomllib")
    except ModuleNotFoundError:  # Python 3.9 and 3.10
        return None
    with open(path, "rb") as handle:
        data: dict[str, Any] = tomllib.load(handle)
    return data


def _sources() -> list[Path]:
    return sorted(SRC.glob("*.py")) + sorted((ROOT / "tests").glob("*.py"))


class VersionTest(unittest.TestCase):
    def test_the_version_is_the_same_in_every_place_it_is_written(self) -> None:
        pyproject = (ROOT / "pyproject.toml").read_text(encoding="utf-8")
        declared = re.search(r'^version = "([^"]+)"$', pyproject, re.M)
        assert declared
        self.assertEqual(carragent.__version__, declared.group(1))
        self.assertRegex(carragent.__version__, r"^\d+\.\d+\.\d+$")
        self.assertEqual(carragent.USER_AGENT, f"carragent-python/{carragent.__version__}")
        plugin = KIT / "plugin.json"
        if plugin.exists():
            self.assertIn(f'"version": "{carragent.__version__}"', plugin.read_text(encoding="utf-8"))


class MetadataTest(unittest.TestCase):
    def setUp(self) -> None:
        data = _toml(ROOT / "pyproject.toml")
        if data is None:
            self.skipTest("tomllib is Python 3.11 and later")
        self.data = data
        self.project: dict[str, Any] = data["project"]

    def test_it_says_what_the_package_is(self) -> None:
        project = self.project
        self.assertEqual(project["name"], "carragent")
        self.assertEqual(project["requires-python"], ">=3.9")
        self.assertEqual(project["license"], "MIT")
        self.assertEqual(project["license-files"], ["LICENSE"])
        self.assertEqual(project["readme"], "README.md")
        self.assertEqual(project["dependencies"], [])
        self.assertNotIn("optional-dependencies", project)
        self.assertNotIn("scripts", project)
        self.assertEqual(project["authors"], [{"name": "Carragent", "email": "admin@carragent.com"}])

    def test_it_builds_with_hatchling_which_knows_spdx_licenses(self) -> None:
        build = self.data["build-system"]
        self.assertEqual(build["build-backend"], "hatchling.build")
        self.assertEqual(build["requires"], ["hatchling>=1.27"])

    def test_its_links(self) -> None:
        self.assertEqual(
            self.project["urls"],
            {
                "Homepage": "https://carragent.com/developers",
                "Documentation": "https://carragent.com/developers#sdks",
                "Repository": "https://github.com/vicenccc/carragent-agent-kit",
            },
        )

    def test_its_classifiers(self) -> None:
        classifiers: list[str] = self.project["classifiers"]
        for minor in range(9, 15):
            self.assertIn(f"Programming Language :: Python :: 3.{minor}", classifiers)
        self.assertIn("Typing :: Typed", classifiers)
        # PEP 639: the license is the SPDX expression above, so a License classifier would contradict it.
        self.assertFalse([c for c in classifiers if c.startswith("License ::")])
        self.assertEqual(len(classifiers), len(set(classifiers)))

    def test_the_files_it_names_exist(self) -> None:
        self.assertTrue((ROOT / "README.md").is_file())
        self.assertTrue((ROOT / "LICENSE").is_file())
        self.assertIn("MIT License", (ROOT / "LICENSE").read_text(encoding="utf-8"))
        wheel = self.data["tool"]["hatch"]["build"]["targets"]["wheel"]
        self.assertEqual(wheel["packages"], ["src/carragent"])

    def test_the_license_is_the_kits(self) -> None:
        kit_license = KIT / "LICENSE"
        if not kit_license.exists():
            self.skipTest("this is not inside the agent kit")
        self.assertEqual((ROOT / "LICENSE").read_bytes(), kit_license.read_bytes())


class TypingTest(unittest.TestCase):
    def test_the_package_says_it_is_typed(self) -> None:
        marker = SRC / "py.typed"
        self.assertTrue(marker.is_file())
        self.assertEqual(marker.read_bytes(), b"")

    def test_the_public_names_exist_once(self) -> None:
        names = carragent.__all__
        self.assertEqual(len(names), len(set(names)))
        for name in names:
            self.assertTrue(hasattr(carragent, name), name)

    def test_every_public_class_is_exported(self) -> None:
        modules: list[ModuleType] = [carragent.client, carragent.errors, carragent.models]
        for module in modules:
            for name, value in vars(module).items():
                if inspect.isclass(value) and value.__module__ == module.__name__ and not name.startswith("_"):
                    self.assertIn(name, carragent.__all__, f"{module.__name__}.{name}")

    def test_every_public_method_and_class_has_a_docstring(self) -> None:
        for name in carragent.__all__:
            value = getattr(carragent, name)
            if inspect.isclass(value) and value.__module__.startswith("carragent"):
                self.assertTrue(inspect.getdoc(value), name)
                for member, function in inspect.getmembers(value, inspect.isfunction):
                    if not member.startswith("_"):
                        self.assertTrue(inspect.getdoc(function), f"{name}.{member}")
        self.assertTrue(inspect.getdoc(carragent))


class SourceTest(unittest.TestCase):
    def test_only_the_standard_library_is_imported(self) -> None:
        allowed = set(getattr(sys, "stdlib_module_names", STDLIB))
        for path in sorted(SRC.glob("*.py")):
            for node in ast.walk(ast.parse(path.read_text(encoding="utf-8"))):
                modules: list[str] = []
                if isinstance(node, ast.Import):
                    modules = [alias.name for alias in node.names]
                elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module:
                    modules = [node.module]
                for module in modules:
                    self.assertIn(module.split(".")[0], allowed, f"{path.name} imports {module}")

    def test_it_is_written_for_python_3_9(self) -> None:
        for path in _sources():
            tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path), feature_version=(3, 9))
            for node in ast.walk(tree):
                # `int | None` in an annotation is evaluated when the function is defined, and Python 3.9 cannot.
                annotations = []
                if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    annotations = [a.annotation for a in node.args.args + node.args.kwonlyargs + node.args.posonlyargs]
                    annotations += [node.returns]
                elif isinstance(node, ast.AnnAssign):
                    annotations = [node.annotation]
                for annotation in annotations:
                    if annotation is None:
                        continue
                    for part in ast.walk(annotation):
                        if isinstance(part, ast.BinOp) and isinstance(part.op, ast.BitOr):
                            line = getattr(node, "lineno", 0)
                            self.fail(f"{path.name}:{line}: use Optional[...] or Union[...], not |")
                if isinstance(node, ast.ImportFrom) and node.module == "typing":
                    newer = NEWER_TYPING.intersection(alias.name for alias in node.names)
                    self.assertFalse(newer, f"{path.name}:{node.lineno}: typing has no {sorted(newer)} on 3.9")


class ReadmeTest(unittest.TestCase):
    """The README is the first thing a user copies from, so what it shows must be real."""

    readme = (ROOT / "README.md").read_text(encoding="utf-8")

    def snippets(self) -> list[str]:
        return re.findall(r"```python\n(.*?)```", self.readme, re.S)

    def test_the_snippets_are_python_and_use_names_that_exist(self) -> None:
        self.assertGreaterEqual(len(self.snippets()), 4)
        for snippet in self.snippets():
            tree = ast.parse(snippet)
            for node in ast.walk(tree):
                if isinstance(node, ast.ImportFrom) and node.module == "carragent":
                    for alias in node.names:
                        self.assertTrue(hasattr(carragent, alias.name), alias.name)
                if (
                    isinstance(node, ast.Call)
                    and isinstance(node.func, ast.Attribute)
                    and isinstance(node.func.value, ast.Name)
                    and node.func.value.id == "client"
                ):
                    method = getattr(carragent.Carragent, node.func.attr, None)
                    self.assertTrue(callable(method), f"client.{node.func.attr}() is not a method")
                    accepted = inspect.signature(method).parameters  # type: ignore[arg-type]
                    for keyword in node.keywords:
                        self.assertIn(keyword.arg, accepted, f"client.{node.func.attr}(…{keyword.arg}=…)")

    def test_the_table_of_methods_matches_the_signatures(self) -> None:
        table = [line for line in self.readme.splitlines() if line.startswith("| `")]
        self.assertGreaterEqual(len(table), 8)
        listed: set[str] = set()
        for line in table:
            for name, arguments in re.findall(r"`(\w+)\(([^`]*)\)`", line.split("|")[1]):
                listed.add(name)
                parameters = inspect.signature(getattr(carragent.Carragent, name)).parameters
                for argument in (part.strip() for part in arguments.split(",") if part.strip()):
                    self.assertIn(argument.rstrip("="), parameters, f"{name}({argument})")
        public = {
            name for name, _ in inspect.getmembers(carragent.Carragent, inspect.isfunction) if not name.startswith("_")
        }
        self.assertEqual(listed, public)

    def test_the_claims_about_limits_are_the_codes(self) -> None:
        self.assertIn("8 MiB", self.readme)
        self.assertEqual(carragent.client._MAX_BODY, 8 * 1024 * 1024)  # pyright: ignore[reportPrivateUsage]
        self.assertIn("timeout=10", self.readme)
        self.assertEqual(inspect.signature(carragent.Carragent).parameters["timeout"].default, 10)
        self.assertIn(f'base_url="{carragent.DEFAULT_BASE_URL}"', self.readme)

    def test_the_package_docstring_example_is_python_too(self) -> None:
        match = re.search(r"Example::\n\n((?:    .*\n|\n)+)", carragent.__doc__ or "")
        assert match
        ast.parse("\n".join(line[4:] for line in match.group(1).splitlines()))


if __name__ == "__main__":
    unittest.main()
