import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type RowData,
  type SortingState,
} from '@tanstack/react-table';
import { useState } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';

export interface DataTableProps<TData extends RowData> {
  data: TData[];
  columns: ColumnDef<TData, any>[];
  initialSorting?: SortingState;
  emptyMessage?: React.ReactNode;
}

export function DataTable<TData extends RowData>({
  data,
  columns,
  initialSorting,
  emptyMessage = 'No results.',
}: DataTableProps<TData>) {
  const [sorting, setSorting] = useState<SortingState>(initialSorting ?? []);
  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  });

  return (
    <Table>
      <TableHead>
        {table.getHeaderGroups().map((hg) => (
          <tr key={hg.id}>
            {hg.headers.map((h) => (
              <TableHeaderCell key={h.id} colSpan={h.colSpan}>
                {h.isPlaceholder
                  ? null
                  : flexRender(h.column.columnDef.header, h.getContext())}
              </TableHeaderCell>
            ))}
          </tr>
        ))}
      </TableHead>
      <TableBody>
        {table.getRowModel().rows.length === 0 ? (
          <tr>
            <TableCell colSpan={columns.length} className='text-center text-foreground-subtle'>
              {emptyMessage}
            </TableCell>
          </tr>
        ) : (
          table.getRowModel().rows.map((row) => (
            <TableRow key={row.id}>
              {row.getVisibleCells().map((c) => (
                <TableCell key={c.id}>
                  {flexRender(c.column.columnDef.cell, c.getContext())}
                </TableCell>
              ))}
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  );
}
