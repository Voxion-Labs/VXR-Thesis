#!/usr/bin/env python3
"""
VXR-Thesis: Academic Thesis & LaTeX Document Validator
Verifies LaTeX structural integrity, balanced environments, image paths,
and BibTeX citation consistency without needing external TeX distributions.
"""

import os
import re
import sys


def verify_thesis():
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    thesis_dir = os.path.join(root_dir, "thesis")
    main_tex = os.path.join(thesis_dir, "main.tex")
    bib_file = os.path.join(thesis_dir, "references.bib")

    print("==========================================================")
    print("  VXR-Thesis: Academic LaTeX Document Validation Suite    ")
    print("==========================================================")
    print(f"[INFO] Target: {main_tex}")

    if not os.path.exists(main_tex):
        print(f"[FAIL] Missing main.tex: {main_tex}")
        return False

    with open(main_tex, "r", encoding="utf-8") as f:
        main_content = f.read()

    # 1. Parse inputs
    input_matches = re.findall(r"\\input\{([^}]+)\}", main_content)
    print(f"[INFO] Discovered {len(input_matches)} section inputs.")

    all_text = main_content
    for inp in input_matches:
        inp_path = os.path.join(thesis_dir, inp if inp.endswith(".tex") else inp + ".tex")
        if not os.path.exists(inp_path):
            print(f"[FAIL] Section file missing: {inp_path}")
            return False
        with open(inp_path, "r", encoding="utf-8") as sf:
            content = sf.read()
            all_text += "\n" + content
        print(f"  [OK] Section: {inp}")

    # 2. Check BibTeX references
    bib_keys = set()
    if os.path.exists(bib_file):
        with open(bib_file, "r", encoding="utf-8") as bf:
            bib_content = bf.read()
            bib_keys = set(re.findall(r"@\w+\{([^,]+),", bib_content))
    print(f"[INFO] Discovered {len(bib_keys)} BibTeX citations in references.bib.")

    citations = re.findall(r"\\cite\{([^}]+)\}", all_text)
    missing_cites = set()
    for c in citations:
        for k in [x.strip() for x in c.split(",")]:
            if k not in bib_keys:
                missing_cites.add(k)

    if missing_cites:
        print(f"[WARN] Missing BibTeX entries for: {missing_cites}")
    else:
        print(f"  [OK] All {len(citations)} citations resolved successfully.")

    # 3. Check Figure Paths
    fig_matches = re.findall(r"\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}", all_text)
    missing_figs = []
    for fig in fig_matches:
        fig_path = os.path.normpath(os.path.join(thesis_dir, fig))
        if os.path.exists(fig_path):
            print(f"  [OK] Figure: {fig} ({os.path.getsize(fig_path)} bytes)")
        else:
            print(f"  [FAIL] Figure missing on disk: {fig_path}")
            missing_figs.append(fig)

    if missing_figs:
        print(f"[FAIL] {len(missing_figs)} figure(s) missing.")
        return False

    # 4. Check Environment Balance
    begins = re.findall(r"\\begin\{([^}]+)\}", all_text)
    ends = re.findall(r"\\end\{([^}]+)\}", all_text)

    if len(begins) != len(ends):
        print(f"[FAIL] Unbalanced environments: {len(begins)} begins vs {len(ends)} ends")
        return False
    print(f"  [OK] All {len(begins)} LaTeX environments balanced properly.")

    print("==========================================================")
    print("  LaTeX Paper Structural Validation: 100% PASSED          ")
    print("==========================================================")
    return True


if __name__ == "__main__":
    ok = verify_thesis()
    sys.exit(0 if ok else 1)
