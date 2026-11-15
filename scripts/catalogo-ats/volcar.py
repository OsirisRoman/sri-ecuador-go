import sys, xlrd
wb = xlrd.open_workbook(sys.argv[1])
for sh in wb.sheets():
    print(f"##HOJA\t{sh.name}\t{sh.nrows}\t{sh.ncols}")
    for r in range(sh.nrows):
        vals = []
        for c in range(sh.ncols):
            cell = sh.cell(r, c)
            v = cell.value
            if cell.ctype == xlrd.XL_CELL_NUMBER and float(v).is_integer():
                v = int(v)
            if cell.ctype == xlrd.XL_CELL_DATE:
                try:
                    v = xlrd.xldate_as_datetime(v, wb.datemode).strftime("%Y-%m-%d")
                except Exception:
                    pass
            vals.append(str(v).replace("\t", " ").replace("\n", " ").strip())
        if any(vals):
            print("\t".join(vals))
