import { useId } from 'react';

const DEFAULT_TIME_ZONES = ['Asia/Shanghai', 'UTC', 'America/Los_Angeles', 'Europe/London'] as const;

type TimeZoneInputProps = {
  value: string;
  onChange: (next: string) => void;

  className?: string;
  placeholder?: string;
  disabled?: boolean;

  id?: string;
  name?: string;
  listId?: string;
  options?: string[];
};

export function TimeZoneInput({ value, onChange, className, placeholder, disabled, id, name, listId, options }: TimeZoneInputProps) {
  const raw = useId();
  const safe = raw.replace(/[^a-zA-Z0-9_-]/g, '');
  const inputId = id || `tz-${safe}`;
  const dataListId = listId || `tz-list-${safe}`;

  const items = (options && options.length ? options : Array.from(DEFAULT_TIME_ZONES)).filter((s) => typeof s === 'string' && s.trim() !== '');

  return (
    <>
      <input
        id={inputId}
        name={name}
        type="text"
        className={className || 'form-control'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder || 'Asia/Shanghai'}
        list={dataListId}
        disabled={disabled}
      />
      <datalist id={dataListId}>
        {items.map((z) => (
          <option key={z} value={z}></option>
        ))}
      </datalist>
    </>
  );
}

