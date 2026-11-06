const GIB = 1024 ** 3
const MIB = 1024 ** 2
const KIB = 1024

const number = (value: number, digits: number) =>
  value.toLocaleString('pt-BR', { minimumFractionDigits: digits, maximumFractionDigits: digits })

export function bytes(n: number): string {
  if (n >= 100 * GIB) return `${number(n / GIB, 0)} GiB`
  if (n >= GIB) return `${number(n / GIB, 1)} GiB`
  if (n >= MIB) return `${number(n / MIB, 0)} MiB`
  if (n >= KIB) return `${number(n / KIB, 0)} KiB`
  return `${n} B`
}

export function age(days: number): string {
  if (days < 0) return ''
  if (days === 0) return 'hoje'
  if (days === 1) return 'há 1 dia'
  if (days < 60) return `há ${days} dias`
  if (days < 730) return `há ${Math.round(days / 30)} meses`
  return `há ${Math.round(days / 365)} anos`
}

export function plural(n: number, one: string, many: string): string {
  return `${n.toLocaleString('pt-BR')} ${n === 1 ? one : many}`
}

export const statusLabel: Record<string, string> = {
  removable: 'Removível',
  candidate: 'Candidato',
  in_use: 'Em uso',
  protected: 'Protegido',
}

export const statusHelp: Record<string, string> = {
  removable: 'Regra explícita diz que pode sair. Já vem selecionado.',
  candidate: 'Provavelmente dispensável. Só sai se você marcar.',
  in_use: 'Usado por um container, pod ou processo. O app recusa limpar.',
  protected: 'Dados pessoais, ambiente de trabalho ou sem regra. O app recusa limpar.',
}

export const groupLabel: Record<string, string> = {
  docker_volumes: 'Volumes Docker',
  docker_images: 'Imagens Docker',
  docker_build_cache: 'Build cache do Docker',
  caches: 'Caches',
  dependencies: 'Dependências de projetos',
  system: 'Sistema',
  games_editors: 'Jogos e editores',
  backups: 'Backups e arquivos grandes',
  gc_stack: 'gc-stack',
  other: 'Outras pastas',
}
