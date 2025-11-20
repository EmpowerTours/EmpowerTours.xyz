
/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/


export interface Project {
  id: string;
  name: string;
  category: string;
  image: string;
  url: string;
  description: string;
  features: string[];
  technologies: string[];
}

export interface Service {
  title: string;
  description: string;
  icon: any; // Lucide icon component
}

export interface ChatMessage {
  role: 'user' | 'model';
  text: string;
  isError?: boolean;
}

export interface Artist {
  name: string;
  image: string;
  day: string;
  genre: string;
}

export enum Section {
  HERO = 'hero',
  SERVICES = 'services',
  PORTFOLIO = 'portfolio',
  CONTACT = 'contact',
}
