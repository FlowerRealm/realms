import {
  getModelLibrarySuggestAdmin,
  type ModelLibrarySuggestResult,
} from '../api/models';
import { GenericSuggestInput } from './GenericSuggestInput';

type ManualModelOption = {
  kind: 'manual';
  id: string;
};

type LibraryModelOption = {
  kind: 'library';
  item: ModelLibrarySuggestResult;
};

export type OpenRouterModelOption = ManualModelOption | LibraryModelOption;

export function OpenRouterModelSuggestInput(props: {
  id: string;
  value: string;
  disabled?: boolean;
  placeholder?: string;
  onChange: (value: string) => void;
  onSelect: (item: OpenRouterModelOption) => void;
}) {
  const { id, value, disabled, placeholder, onChange, onSelect } = props;

  return (
    <GenericSuggestInput<OpenRouterModelOption>
      id={id}
      value={value}
      disabled={disabled}
      placeholder={placeholder}
      inputClassName="font-monospace"
      minWidth={460}
      maxWidth={720}
      emptyText="无匹配模型"
      onChange={onChange}
      onSelect={onSelect}
      fetchItems={async (q) => {
        const res = await getModelLibrarySuggestAdmin(q, 20);
        if (!res.success) throw new Error(res.message || '加载失败');
        return (res.data || []).map((item) => ({ kind: 'library', item }));
      }}
      localItems={(q) => {
        const trimmed = q.trim();
        if (!trimmed) return [];
        return [{ kind: 'manual', id: trimmed }];
      }}
      getItemKey={(item) => (item.kind === 'manual' ? `manual:${item.id}` : `library:${item.item.id}`)}
      renderItem={(item) => {
        if (item.kind === 'manual') {
          return (
            <>
              <div className="small fw-semibold text-truncate">使用当前输入值</div>
              <div className="text-muted smaller font-monospace text-truncate">{item.id}</div>
            </>
          );
        }
        const model = item.item;
        return (
          <>
            <div className="small fw-semibold font-monospace text-truncate">{model.id}</div>
            <div className="text-muted smaller text-truncate">
              {model.name || model.id}
              {model.owned_by ? ` · ${model.owned_by}` : ''}
            </div>
          </>
        );
      }}
    />
  );
}
