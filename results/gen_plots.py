#!/usr/bin/env python3
"""Generate benchmark comparison plots from result files."""

import re
import os
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

RESULTS_DIR = os.path.dirname(os.path.abspath(__file__))
FILES = {
    1: os.path.join(RESULTS_DIR, 'bench-cpu1.txt'),
    4: os.path.join(RESULTS_DIR, 'bench-cpu4.txt'),
    8: os.path.join(RESULTS_DIR, 'bench-cpu8.txt'),
    12: os.path.join(RESULTS_DIR, 'bench-cpu12.txt'),
}

LIBRARY_MAP = {
    'SyncMap': 'sync.Map',
    'XsyncMapOf': 'xsync.Map',
    'CornelkHashmap': 'cornelk/hashmap',
    'Haxmap': 'haxmap',
    'OrcamanCmap': 'orcaman/cmap',
}

LIBRARY_ORDER = ['sync.Map', 'xsync.Map', 'cornelk/hashmap', 'haxmap', 'orcaman/cmap']

COLORS = {
    'sync.Map': '#1f77b4',
    'xsync.Map': '#d62728',
    'cornelk/hashmap': '#2ca02c',
    'haxmap': '#9467bd',
    'orcaman/cmap': '#8c564b',
}

MARKERS = {
    'sync.Map': 'o',
    'xsync.Map': 's',
    'cornelk/hashmap': '^',
    'haxmap': 'D',
    'orcaman/cmap': 'v',
}

LINE = re.compile(
    r'Benchmark(\w+?)_(WarmUp|NoWarmUp)_(StringKeys|IntKeys)/size=(\d+)/reads=(\d+)%(?:-\d+)?\s+'
    r'\d+\s+[\d.]+ ns/op\s+([\d.]+) ops/s'
)

LINE_RANGE = re.compile(
    r'Benchmark(\w+?)_Range_(StringKeys|IntKeys)/size=(\d+)(?:-\d+)?\s+'
    r'\d+\s+[\d.]+ ns/op\s+([\d.]+) ops/s'
)

SIZE_LABELS = {100: '100', 1000: '1K', 100000: '100K', 1000000: '1M'}

def parse_results():
    """Parse all result files into nested dicts."""
    data = {}  # (variant, keytype, size, reads) -> {lib -> {gomaxprocs -> ops_s}}
    range_data = {}  # (keytype, size) -> {lib -> {gomaxprocs -> ops_s}}
    for procs, path in FILES.items():
        with open(path) as f:
            for line in f:
                m = LINE.search(line)
                if m:
                    lib_raw, variant, keytype, size, reads, ops_s = m.groups()
                    lib = LIBRARY_MAP.get(lib_raw)
                    if lib is None:
                        continue
                    key = (variant, keytype, int(size), int(reads))
                    data.setdefault(key, {}).setdefault(lib, {})[procs] = float(ops_s)
                    continue
                m = LINE_RANGE.search(line)
                if m:
                    lib_raw, keytype, size, ops_s = m.groups()
                    lib = LIBRARY_MAP.get(lib_raw)
                    if lib is None:
                        continue
                    key = (keytype, int(size))
                    range_data.setdefault(key, {}).setdefault(lib, {})[procs] = float(ops_s)
    return data, range_data

def format_ops(x, _):
    """Format ops/s values with M suffix."""
    if x >= 1e9:
        return f'{x/1e9:.1f}B'
    if x >= 1e6:
        return f'{x/1e6:.0f}M'
    if x >= 1e3:
        return f'{x/1e3:.0f}K'
    return f'{x:.0f}'

def make_figure(data, variant, keytype, read_pcts, sizes):
    """Create a composite figure with subplots: rows=read_pct, cols=size."""
    nrows = len(read_pcts)
    ncols = len(sizes)
    fig, axes = plt.subplots(nrows, ncols, figsize=(4.2 * ncols, 3.5 * nrows),
                              squeeze=False)

    procs_list = sorted(FILES.keys())

    for ri, reads in enumerate(read_pcts):
        for ci, size in enumerate(sizes):
            ax = axes[ri][ci]
            key = (variant, keytype, size, reads)
            bench_data = data.get(key, {})

            for lib in LIBRARY_ORDER:
                if lib not in bench_data:
                    continue
                lib_data = bench_data[lib]
                xs = [p for p in procs_list if p in lib_data]
                ys = [lib_data[p] for p in xs]
                ax.plot(xs, ys, color=COLORS[lib], marker=MARKERS[lib],
                        markersize=6, linewidth=1.8, label=lib)

            ax.set_xticks(procs_list)
            ax.yaxis.set_major_formatter(ticker.FuncFormatter(format_ops))
            ax.grid(True, alpha=0.3)
            ax.set_title(f'size={SIZE_LABELS[size]}, reads={reads}%', fontsize=10)

            if ri == nrows - 1:
                ax.set_xlabel('GOMAXPROCS')
            if ci == 0:
                ax.set_ylabel('ops/s')

    # Shared legend at the bottom
    handles, labels = axes[0][0].get_legend_handles_labels()
    fig.legend(handles, labels, loc='lower center', ncol=len(LIBRARY_ORDER),
               fontsize=9, bbox_to_anchor=(0.5, -0.02))

    variant_label = 'Warm-up' if variant == 'WarmUp' else 'No warm-up'
    keytype_label = 'string keys' if keytype == 'StringKeys' else 'int keys'
    fig.suptitle(f'{variant_label}, {keytype_label}', fontsize=14, fontweight='bold')
    fig.tight_layout(rect=[0, 0.04, 1, 0.97])
    return fig

def make_range_figure(range_data, keytype, sizes):
    """Create a single-row figure for range benchmarks: one subplot per size."""
    ncols = len(sizes)
    fig, axes = plt.subplots(1, ncols, figsize=(4.2 * ncols, 3.5), squeeze=False)

    procs_list = sorted(FILES.keys())

    for ci, size in enumerate(sizes):
        ax = axes[0][ci]
        key = (keytype, size)
        bench_data = range_data.get(key, {})

        for lib in LIBRARY_ORDER:
            if lib not in bench_data:
                continue
            lib_data = bench_data[lib]
            xs = [p for p in procs_list if p in lib_data]
            ys = [lib_data[p] for p in xs]
            ax.plot(xs, ys, color=COLORS[lib], marker=MARKERS[lib],
                    markersize=6, linewidth=1.8, label=lib)

        ax.set_xticks(procs_list)
        ax.yaxis.set_major_formatter(ticker.FuncFormatter(format_ops))
        ax.grid(True, alpha=0.3)
        ax.set_title(f'size={SIZE_LABELS[size]}', fontsize=10)
        ax.set_xlabel('GOMAXPROCS')
        if ci == 0:
            ax.set_ylabel('ops/s')

    handles, labels = axes[0][0].get_legend_handles_labels()
    fig.legend(handles, labels, loc='lower center', ncol=len(LIBRARY_ORDER),
               fontsize=9, bbox_to_anchor=(0.5, -0.04))

    keytype_label = 'string keys' if keytype == 'StringKeys' else 'int keys'
    fig.suptitle(f'Range under contention, {keytype_label}', fontsize=14, fontweight='bold')
    fig.tight_layout(rect=[0, 0.06, 1, 0.95])
    return fig

def main():
    data, range_data = parse_results()
    sizes = [100, 1000, 100000, 1000000]

    configs = [
        ('WarmUp', 'StringKeys', [100, 99, 90, 75]),
        ('WarmUp', 'IntKeys', [100, 99, 90, 75]),
        ('NoWarmUp', 'StringKeys', [99, 90, 75]),
        ('NoWarmUp', 'IntKeys', [99, 90, 75]),
    ]

    for variant, keytype, read_pcts in configs:
        fig = make_figure(data, variant, keytype, read_pcts, sizes)
        fname = f'{variant.lower()}_{keytype.lower()}.png'
        path = os.path.join(RESULTS_DIR, fname)
        fig.savefig(path, dpi=150, bbox_inches='tight', facecolor='white')
        plt.close(fig)
        print(f'Saved {path}')

    for keytype in ['StringKeys', 'IntKeys']:
        fig = make_range_figure(range_data, keytype, sizes)
        fname = f'range_{keytype.lower()}.png'
        path = os.path.join(RESULTS_DIR, fname)
        fig.savefig(path, dpi=150, bbox_inches='tight', facecolor='white')
        plt.close(fig)
        print(f'Saved {path}')

if __name__ == '__main__':
    main()
