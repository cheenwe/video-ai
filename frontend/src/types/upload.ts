export type UploadTaskStatus = 'pending' | 'uploading' | 'done' | 'error'

export interface UploadTask {
  id: string
  name: string
  size: number
  storageId: string
  path: string
  file: File
  relativePath?: string
  progress: number
  status: UploadTaskStatus
  error?: string
}
